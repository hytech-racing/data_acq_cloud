package background

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type FileProcessor struct {
	uploadDir               string
	queueChan               chan *FileJob
	stopChan                chan bool
	processingWg            sync.WaitGroup
	activelyProcessing      bool
	mu                      sync.RWMutex
	MiddlewareEstimatedSize atomic.Int64
	TotalSize               atomic.Int64
	maxTotalSize            int64
	dbClient                *database.DatabaseClient
	s3Repository            *s3.S3Repository
}

type FileJob struct {
	ID        string
	Filename  string
	Size      int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	FilePath  string
	FileDir   string
	Date      time.Time
}

func NewFileProcessor(uploadDir string, maxTotalSize int64, dbClient *database.DatabaseClient, s3Repository *s3.S3Repository) (*FileProcessor, error) {
	err := os.MkdirAll(uploadDir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %v", err)
	}

	fp := &FileProcessor{
		uploadDir:    uploadDir,
		queueChan:    make(chan *FileJob, 100),
		stopChan:     make(chan bool),
		processingWg: sync.WaitGroup{},
		mu:           sync.RWMutex{},
		maxTotalSize: maxTotalSize,
		dbClient:     dbClient,
		s3Repository: s3Repository,
	}

	var totalSize int64
	err = filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	fp.TotalSize.Store(totalSize)
	return fp, nil
}

func (fp *FileProcessor) QueueFile(fileHeader *multipart.FileHeader) (*FileJob, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	id := fmt.Sprintf("job_%d", time.Now().UnixNano())
	job := &FileJob{
		ID:        id,
		Filename:  fileHeader.Filename,
		Size:      fileHeader.Size,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		FilePath:  filepath.Join(fp.uploadDir, fmt.Sprintf("%s_%s", id, fileHeader.Filename)),
		FileDir:   fp.uploadDir,
		Date:      time.Now(), // TODO: Change to date gotten from MCAP
	}

	dst, err := os.Create(job.FilePath)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		os.Remove(job.FilePath)
		return nil, err
	}

	fp.TotalSize.Add(job.Size)
	log.Printf("job put in queue, %v", job.ID)
	fp.queueChan <- job

	return job, nil
}

func (fp *FileProcessor) jobQueueListener(ctx context.Context) {
	defer fp.processingWg.Done()

	for {
		// Ensures that only 1 file is being processed at a time (to save resources)
		// And that a file currently being processed tries to finish
		if fp.activelyProcessing {
			time.Sleep(5 * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-fp.stopChan:
			return
		case job := <-fp.queueChan:
			if err := fp.processFileJob(job); err != nil {
				log.Printf("Failed to process file %s: %v", job.Filename, err)
				fp.updateJobStatus(job, StatusFailed)
				// TODO: Add job status to database
			}
		}
	}
}

func (fp *FileProcessor) processFileJob(job *FileJob) error {
	log.Printf("Starting job %v", job.ID)
	ctx := context.TODO()
	fp.setCurrentlyProcessing(true)
	fp.updateJobStatus(job, StatusProcessing)

	// mcapFile processing logic here
	mcapFile, err := os.Open(job.FilePath)
	if err != nil {
		return fmt.Errorf("could not open mcapFile %v, received error %v", job.Filename, err)
	}
	defer mcapFile.Close()
	log.Printf("Opened mcapFile %v", job.Filename)

	mcapUtils := utils.NewMcapUtils()

	reader, err := mcapUtils.NewReader(mcapFile)
	if err != nil {
		return fmt.Errorf("could not create mcap reader: %v", err)
	}

	info, err := reader.Info()
	if err != nil {
		return fmt.Errorf("could not get info for mcap reader: %v", err)
	}

	err = mcapUtils.LoadAllSchemas(info)
	if err != nil {
		return err
	}

	schemaList, err := mcapUtils.GetMcapSchemaList(reader)
	if err != nil {
		return err
	}

	message_iterator, err := reader.Messages()
	if err != nil {
		return fmt.Errorf("could not get mcap mesages: %v", err)
	}

	// This is all the subsribers relavent to handling an MCAP mcapFile. You can attach more workers here if need be.
	subscriberMapping := make(map[string]messaging.SubscriberFunc)
	subscriberMapping["vn_plot"] = messaging.PlotLatLon
	subscriberMapping["matlab_writer"] = messaging.CreateRawMatlabFile

	publisher := messaging.NewPublisher(true)
	subscriber_names := make([]string, len(subscriberMapping))
	idx := 0
	for subscriber_name, function := range subscriberMapping {
		subscriber_names[idx] = subscriber_name
		publisher.Subscribe(idx+1, subscriber_name, function)
		idx++
	}

	log.Printf("Starting subsribers for job: %s", job.ID)
	go func() {
		// Some subscribers may need specfic information before being able to perform their tasks. For example, (CreateInterpolatedMatlabFile)
		// Because of this, they will need their first message to set paramaters. This is what initMessage is for.
		initMessage := make(map[string]interface{})
		initMessage["schema_list"] = schemaList
		fp.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage}, &subscriber_names)

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				fp.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.EOF}, &subscriber_names)
				break
			}

			if err != nil {
				log.Printf("error reading mcap message: %v", err)
				return
			}

			if schema == nil {
				log.Printf("no schema found for channel ID: %d, channel: %v", message.ChannelID, channel)
				continue
			}

			decodedMessage, err := mcapUtils.GetDecodedMessage(schema, message)
			if err != nil {
				log.Printf("could not decode message: %v", err)
				continue
			}

			fp.routeMessagesToSubscribers(ctx, publisher, decodedMessage, &subscriber_names)
		}

		// Need to make sure to close the subscribers or our code will hang and wait forever
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	log.Printf("All subscribers finished for job %v", job.ID)

	results := publisher.Results()
	var rawMatlabData *map[string]map[string]interface{}
	if outer, ok := results["matlab_writer"]; ok {
		if data, ok := outer.ResultData["raw_data"]; ok {
			rawMatlabData = data.(*map[string]map[string]interface{})
		}
	}

	var vnLatLonPlotWriter *io.WriterTo
	if outer, ok := results["vn_plot"]; ok {
		if data, ok := outer.ResultData["writer_to"]; ok {
			vnLatLonPlotWriter = data.(*io.WriterTo)
		}
	}

	genericFileName := strings.Split(job.Filename, ".")[0]
	matFileName := fmt.Sprintf("%s.mat", genericFileName)
	matFilePath := filepath.Join(job.FileDir, matFileName)
	err = utils.CreateMatlabFile(matFilePath, rawMatlabData)
	if err != nil {
		log.Fatal(err)
	}

	mcapFileS3Reader, err := os.Open(job.FilePath)
	if err != nil {
		log.Fatalf("could not open mcap file %v", job.FilePath)
	}
	defer mcapFileS3Reader.Close()

	year, month, day := job.Date.Date()
	mcapFileName := job.Filename
	mcapObjectFilePath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, mcapFileName)
	err = fp.s3Repository.WriteObjectReader(ctx, mcapFileS3Reader, mcapObjectFilePath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded mcap file %v to s3", mcapFileName)

	matFile, err := os.Open(matFilePath)
	if err != nil {
		log.Fatalf("could not open mat matFile: %v", err)
	}

	matObjectFilePath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, matFileName)
	err = fp.s3Repository.WriteObjectReader(ctx, matFile, matObjectFilePath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("uploaded mat file %v to s3", matFileName)

	vnLatLonPlotName := fmt.Sprintf("%v.png", genericFileName)
	vnLatLonPlotFilePath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, vnLatLonPlotName)
	err = fp.s3Repository.WriteObjectWriterTo(ctx, vnLatLonPlotWriter, vnLatLonPlotFilePath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("uploaded vn lat lon plot %v to s3", vnLatLonPlotName)

	if err := os.Remove(matFilePath); err != nil {
		return fmt.Errorf("failed to remove created mat mcapFile: %w", err)
	}

	// After successful processing, remove the mcapFile and update total size
	if err := os.Remove(job.FilePath); err != nil {
		return fmt.Errorf("failed to remove processed mcapFile: %w", err)
	}

	mcapFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  mcapObjectFilePath,
		FileName:  mcapFileName,
	}
	mcapFiles := make([]models.FileModel, 1)
	mcapFiles = append(mcapFiles, mcapFileEntry)

	matFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  matObjectFilePath,
		FileName:  matFileName,
	}
	matFiles := make([]models.FileModel, 1)
	matFiles = append(matFiles, matFileEntry)

	vehicleRunModel := &models.VehicleRunModel{
		Date:      job.Date,
		McapFiles: mcapFiles,
		MatFiles:  matFiles,
	}

	_, err = fp.dbClient.VehicleRunUseCase().CreateVehicleRun(ctx, vehicleRunModel)
	if err != nil {
		log.Fatal(err)
	}

	fp.TotalSize.Add(-job.Size)
	fp.MiddlewareEstimatedSize.Add(-job.Size)
	fp.updateJobStatus(job, StatusCompleted)
	fp.setCurrentlyProcessing(false)

	log.Printf("Completed job %v", job.ID)
	return nil
}

func (fp *FileProcessor) routeMessagesToSubscribers(ctx context.Context, publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, *allNames...)
	case "hytech_msgs.VNData":
		subscriberNames = append(subscriberNames, "vn_plot", "matlab_writer")
	default:
		subscriberNames = append(subscriberNames, "matlab_writer")
	}

	publisher.Publish(ctx, decodedMessage, subscriberNames)
}

func (fp *FileProcessor) updateJobStatus(job *FileJob, status string) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	job.Status = status
	job.UpdatedAt = time.Now()
}

func (fp *FileProcessor) setCurrentlyProcessing(flag bool) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.activelyProcessing = flag
}

func (fp *FileProcessor) Start(ctx context.Context) {
	fp.processingWg.Add(1)
	go fp.jobQueueListener(ctx)
}

func (fp *FileProcessor) Stop() {
	close(fp.stopChan)
	fp.processingWg.Wait()
}

func (fp *FileProcessor) MaxTotalSize() int64 {
	return fp.maxTotalSize
}

func (fp *FileProcessor) syncTotalSize() {
	for {
		time.Sleep(1 * time.Minute)
		fp.MiddlewareEstimatedSize.CompareAndSwap(fp.MiddlewareEstimatedSize.Load(), fp.TotalSize.Load())
	}
}
