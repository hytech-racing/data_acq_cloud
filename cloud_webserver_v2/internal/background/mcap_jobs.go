package background

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

type PostProcessMCAPJob struct {
	fileJob *FileJob
}

func (job *PostProcessMCAPJob) JobID() string {
	return job.fileJob.ID
}

func ProcessMcapUploadJob(fp *FileProcessor, job *FileJob) error {
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
	genericFileName := strings.Split(job.Filename, ".")[0]

	log.Printf("Starting subsribers for job: %s", job.ID)
	go func() {
		// Some subscribers may need specfic information before being able to perform their tasks. For example, (CreateInterpolatedMatlabFile)
		// Because of this, they will need their first message to set paramaters. This is what initMessage is for.
		initMessage := make(map[string]interface{})
		initMessage["schema_list"] = schemaList
		initMessage["file_name"] = genericFileName
		initMessage["file_path"] = job.FileDir
		routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage}, &subscriber_names)

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.EOF}, &subscriber_names)
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

			routeMessagesToSubscribers(ctx, publisher, decodedMessage, &subscriber_names)
		}

		// Need to make sure to close the subscribers or our code will hang and wait forever
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	log.Printf("All subscribers finished for job %v", job.ID)

	results := publisher.Results()

	var hdf5Location string
	if outer, ok := results["matlab_writer"]; ok {
		if data, ok := outer.ResultData["file_path"]; ok {
			hdf5Location = data.(string)
		}
	}

	var vnLatLonPlotWriter *io.WriterTo
	if outer, ok := results["vn_plot"]; ok {
		if data, ok := outer.ResultData["writer_to"]; ok {
			vnLatLonPlotWriter = data.(*io.WriterTo)
		}
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

	hdf5File, err := os.Open(hdf5Location)
	if err != nil {
		log.Fatalf("could not open mat matFile: %v", err)
	}

	hdf5FileName := fmt.Sprintf("%s.h5", genericFileName)
	matObjectFilePath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, hdf5FileName)
	err = fp.s3Repository.WriteObjectReader(ctx, hdf5File, matObjectFilePath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("uploaded hdf5 file %v to s3", hdf5FileName)

	vnLatLonPlotName := fmt.Sprintf("%v.png", genericFileName)
	vnLatLonPlotFileObjectPath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, vnLatLonPlotName)
	err = fp.s3Repository.WriteObjectWriterTo(ctx, vnLatLonPlotWriter, vnLatLonPlotFileObjectPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("uploaded vn lat lon plot %v to s3", vnLatLonPlotName)

	if err := os.Remove(hdf5Location); err != nil {
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
	mcapFiles[0] = mcapFileEntry

	matFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  matObjectFilePath,
		FileName:  hdf5FileName,
	}
	matFiles := make([]models.FileModel, 1)
	matFiles[0] = matFileEntry

	contentFiles := make(map[string][]models.FileModel)
	vnPlotFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  vnLatLonPlotFileObjectPath,
		FileName:  vnLatLonPlotName,
	}
	vnPlotFiles := []models.FileModel{vnPlotFileEntry}
	contentFiles["vn_lat_lon_plot"] = vnPlotFiles

	vehicleRunModel := &models.VehicleRunModel{
		Date:         job.Date,
		CarModel:     "HT08",
		McapFiles:    mcapFiles,
		MatFiles:     matFiles,
		ContentFiles: contentFiles,
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

func routeMessagesToSubscribers(ctx context.Context, publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
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
