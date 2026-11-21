package background

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	VN_LATLON_PLOT_KEY  = "vn_lat_lon_plot"
	VN_TIMEVEL_PLOT_KEY = "vn_time_vel_plot"
	RUN_METADATA_DIR    = "/data/run_metadata"
)

type mcapConversionOutputs struct {
	MatLocation        string
	MatObjectPath      string
	LatLonObjectPath   string
	VelocityObjectPath string
}

// PostProcessMCAPUploadJob handles the post processing of MCAP files.
// PostProcessMCAPUploadJob serves as a wrapper struct to hold the Process function
// so it implicitely inherits FileJobProcessor.
type PostProcessMCAPUploadJob struct {
	// RerunOf updates this existing run instead of creating one. It is nil for uploads.
	RerunOf *primitive.ObjectID
}

func (p *PostProcessMCAPUploadJob) ProcessFileJob(fp *FileProcessor, job *FileJob) error {
	isRerun := p.RerunOf != nil
	ctx := context.Background()
	fp.broadcastFileUploadStart(job)
	defer fp.finishJob(job)

	durationSeconds, fileHash, err := fp.downloadAndStageMCAP(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to download directly uploaded mcap %v: %w", job.Filename, err)
	}

	recordId := primitive.NewObjectID()
	if isRerun {
		recordId = *p.RerunOf
		// Fail before converting if the run was deleted after the job was queued.
		if _, err := fp.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, recordId); err != nil {
			return fmt.Errorf("could not find vehicle run %v to re-convert: %w", recordId.Hex(), err)
		}
	}

	mcapFileName := job.Filename
	mcapObjectFilePath := job.SourceS3Key

	// The vehicle run is created upfront with only the MCAP file attached to it. Converted
	// files (the MAT file and its plots) are attached later, but only when the conversion
	// succeeds. This makes sure an uploaded MCAP is always returned by the fetch endpoints.
	// A rerun discards this model and reloads the existing run after conversion.
	vehicleRunModel := &models.VehicleRunModel{
		Date:     job.Date,
		CarModel: "HT09",
		McapFiles: []models.FileModel{
			{
				AwsBucket:       fp.s3Repository.Bucket(),
				FilePath:        mcapObjectFilePath,
				FileName:        mcapFileName,
				FileHash:        fileHash,
				DurationSeconds: durationSeconds,
			},
		},
		Id:     recordId,
		Hidden: job.Hidden,
	}

	// A rerun leaves the run untouched on failure, while an upload still saves the MCAP-only run.
	fail := func(conversionErr error) error {
		if !isRerun {
			_, err := fp.dbClient.VehicleRunUseCase().CreateVehicleRun(ctx, vehicleRunModel)
			if err != nil {
				return fmt.Errorf("failed to save mcap-only vehicle run for %v: %w", job.Filename, err)
			}
			log.Printf("saved vehicle run %v with only the mcap file: %v", vehicleRunModel.Id.Hex(), conversionErr)
		}
		return conversionErr
	}

	genericFileName := strings.Split(job.Filename, ".")[0]
	mcapResults, err := p.readMCAPMessages(ctx, job, job.ID)
	if err != nil {
		return fail(err)
	}

	var matLocation string
	if outer, ok := mcapResults[messaging.MAT]; ok {
		if data, ok := outer.ResultData["file_path"]; ok {
			matLocation = data.(string)
		}
	}
	if matLocation == "" {
		return fail(fmt.Errorf("no mat file was generated for %v", job.Filename))
	}
	defer func() {
		if err := os.Remove(matLocation); err != nil {
			log.Printf("failed to remove created MAT file: %v", err)
		}
	}()

	// Extracting VN Lat-Lon file location from results
	var vnLatLonPlotWriter *io.WriterTo
	if outer, ok := mcapResults[messaging.LATLON]; ok {
		if data, ok := outer.ResultData["writer_to"]; ok {
			vnLatLonPlotWriter = data.(*io.WriterTo)
		}
	}

	// Extracting VN Vel file location from results
	var vnTimeVelPlotWriter *io.WriterTo
	if outer, ok := mcapResults[messaging.VELOCITY]; ok {
		if data, ok := outer.ResultData["writer_to"]; ok {
			vnTimeVelPlotWriter = data.(*io.WriterTo)
		}
	}

	matFile, err := os.Open(matLocation)
	if err != nil {
		return fail(fmt.Errorf("could not open generated mat file %v: %w", matLocation, err))
	}
	defer matFile.Close()

	matFileName := fmt.Sprintf("%s.mat", genericFileName)
	matObjectFilePath := fmt.Sprintf("%s/%s", recordId.Hex(), matFileName)
	if err := fp.s3Repository.PutObject(ctx, matFile, matObjectFilePath); err != nil {
		return fail(fmt.Errorf("failed to upload mat file %v to s3: %w", matFileName, err))
	}
	log.Printf("uploaded mat file %v to s3", matFileName)

	// Uploading Lat-Lon file to S3
	if vnLatLonPlotWriter == nil {
		return fail(fmt.Errorf("no lat-lon plot could be generated for %v", job.Filename))
	}
	vnLatLonPlotName := fmt.Sprintf("%v_LatLon.png", genericFileName)
	vnLatLonPlotFileObjectPath := fmt.Sprintf("%s/%s", recordId.Hex(), vnLatLonPlotName)
	if err := fp.s3Repository.PutObjectWithWriterTo(ctx, vnLatLonPlotWriter, vnLatLonPlotFileObjectPath); err != nil {
		return fail(fmt.Errorf("failed to upload vn lat lon plot %v to s3: %w", vnLatLonPlotName, err))
	}
	log.Printf("uploaded vn lat lon plot %v to s3", vnLatLonPlotName)

	// Uploading Time-Vel file to S3
	if vnTimeVelPlotWriter == nil {
		return fail(fmt.Errorf("no time-velocity plot could be generated for %v", job.Filename))
	}
	vnTimeVelPlotName := fmt.Sprintf("%v_Velocity.png", genericFileName)
	vnTimeVelPlotFileObjectPath := fmt.Sprintf("%s/%s", recordId.Hex(), vnTimeVelPlotName)
	if err := fp.s3Repository.PutObjectWithWriterTo(ctx, vnTimeVelPlotWriter, vnTimeVelPlotFileObjectPath); err != nil {
		return fail(fmt.Errorf("failed to upload vn time vel plot %v to s3: %w", vnTimeVelPlotName, err))
	}
	log.Printf("uploaded vn time vel plot %v to s3", vnTimeVelPlotName)

	outputs := &mcapConversionOutputs{
		MatLocation:        matLocation,
		MatObjectPath:      matObjectFilePath,
		LatLonObjectPath:   vnLatLonPlotFileObjectPath,
		VelocityObjectPath: vnTimeVelPlotFileObjectPath,
	}

	if err := pushToArchive(job, outputs); err != nil {
		log.Printf("Error when archiving file: %v", err)
	}

	if isRerun {
		// Reload the run so metadata edits made while the conversion was running survive.
		vehicleRunModel, err = fp.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, recordId)
		if err != nil {
			return fmt.Errorf("could not load vehicle run %v after re-conversion: %w", recordId.Hex(), err)
		}
	}

	bucket := fp.s3Repository.Bucket()
	vehicleRunModel.MatFiles = []models.FileModel{convertedFile(bucket, outputs.MatObjectPath)}
	if vehicleRunModel.ContentFiles == nil {
		vehicleRunModel.ContentFiles = make(map[string][]models.FileModel)
	}
	vehicleRunModel.ContentFiles[VN_LATLON_PLOT_KEY] = []models.FileModel{convertedFile(bucket, outputs.LatLonObjectPath)}
	vehicleRunModel.ContentFiles[VN_TIMEVEL_PLOT_KEY] = []models.FileModel{convertedFile(bucket, outputs.VelocityObjectPath)}

	if isRerun {
		err = fp.dbClient.VehicleRunUseCase().UpdateVehicleRun(ctx, recordId, vehicleRunModel)
	} else {
		_, err = fp.dbClient.VehicleRunUseCase().CreateVehicleRun(ctx, vehicleRunModel)
	}
	if err != nil {
		return fmt.Errorf("failed to save vehicle run for %v: %w", job.Filename, err)
	}

	fp.broadcastFileUploadEnd(job, ctx, vehicleRunModel)

	log.Printf("Completed job %v", job.ID)
	return nil
}

func convertedFile(bucket, objectPath string) models.FileModel {
	return models.FileModel{
		AwsBucket: bucket,
		FilePath:  objectPath,
		FileName:  path.Base(objectPath),
	}
}

func pushToArchive(job *FileJob, outputs *mcapConversionOutputs) error {
	if os.Getenv("ENV") != "PRODUCTION" {
		return nil
	}

	// Directly uploaded MCAPs keep the S3 key the client wrote to, which has more
	// path segments than the record ID used for generated files. Create the parent
	// of each destination rather than assuming one layout.
	matVolumePath := filepath.Join(RUN_METADATA_DIR, outputs.MatObjectPath)
	mcapVolumePath := filepath.Join(RUN_METADATA_DIR, job.SourceS3Key)
	if err := os.MkdirAll(filepath.Dir(matVolumePath), 0o755); err != nil {
		return fmt.Errorf("failed to create the run metadata directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(mcapVolumePath), 0o755); err != nil {
		return fmt.Errorf("failed to create the run metadata directory: %w", err)
	}

	srcMatFile, err := os.Open(outputs.MatLocation)
	if err != nil {
		return fmt.Errorf("failed to open mat file for the volume: %w", err)
	}
	defer srcMatFile.Close()

	destMatFile, err := os.Create(matVolumePath)
	if err != nil {
		return fmt.Errorf("error to create mat file in volume %w", err)
	}
	defer destMatFile.Close()

	if _, err := io.Copy(destMatFile, srcMatFile); err != nil {
		os.Remove(matVolumePath)
		return fmt.Errorf("failed to copy mat file over to volume: %w", err)
	}

	// A job that reused the archived copy already has it at the destination, so copying
	// would truncate the source before anything reads it.
	if filepath.Clean(job.FilePath) != filepath.Clean(mcapVolumePath) {
		volumeMcapFile, err := os.Open(job.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open mcap file for the volume: %w", err)
		}
		defer volumeMcapFile.Close()

		// Create the MCAP file in the volume
		destMcapFile, err := os.Create(mcapVolumePath)
		if err != nil {
			return fmt.Errorf("error to create mcap file in volume %w", err)
		}
		defer destMcapFile.Close()

		// Copy the MCAP file contents over to the file in the volume
		if _, err := io.Copy(destMcapFile, volumeMcapFile); err != nil {
			return fmt.Errorf("failed to copy mcap file over to volume: %v", err)
		}
	}

	return nil
}

func (p *PostProcessMCAPUploadJob) readMCAPMessages(ctx context.Context, job *FileJob, matFileName string) (messaging.SubscriberResults, error) {
	// This is all the subsribers relavent to handling an MCAP mcapFile. You can attach more workers here if need be.
	subscriberMapping := make(map[string]messaging.SubscriberFunc)
	subscriberMapping[messaging.LATLON] = messaging.PlotLatLon
	subscriberMapping[messaging.VELOCITY] = messaging.PlotTimeVelocity
	subscriberMapping[messaging.MAT] = messaging.CreateRawMatFile

	return p.publishMCAPMessages(ctx, job, matFileName, subscriberMapping)
}

// conversionMu serializes conversions because libhdf5 links a C library built
// without thread safety. One conversion at a time also bounds its CPU and memory.
var conversionMu sync.Mutex

// The whole subscriber run is serialized, including the MAT file creation and close inside its subscriber.
func (p *PostProcessMCAPUploadJob) publishMCAPMessages(
	ctx context.Context,
	job *FileJob,
	matFileName string,
	subscriberMapping map[string]messaging.SubscriberFunc,
) (messaging.SubscriberResults, error) {
	conversionMu.Lock()
	defer conversionMu.Unlock()

	// mcapFile processing logic here
	mcapFile, err := os.Open(job.FilePath)
	if err != nil {
		return nil, fmt.Errorf("could not open mcapFile %v, received error %v", job.Filename, err)
	}
	defer mcapFile.Close()
	log.Printf("Opened mcapFile %v", job.Filename)

	mcapUtils := utils.NewMcapUtils()

	mcapReader, err := mcapUtils.NewReader(mcapFile)
	if err != nil {
		return nil, fmt.Errorf("could not create mcap reader: %v", err)
	}

	message_iterator, err := mcapReader.Reader.Messages()
	if err != nil {
		return nil, fmt.Errorf("could not get mcap mesages: %v", err)
	}

	publisher := messaging.NewPublisher().WithRouter(routeMCAPDecodedMessage).WithResultsListener()
	subscriber_names := make([]string, len(subscriberMapping))
	idx := 0
	for subscriber_name, function := range subscriberMapping {
		subscriber_names[idx] = subscriber_name
		publisher.Subscribe(idx+1, subscriber_name, function)
		idx++
	}

	log.Printf("Starting subsribers for job: %s", job.ID)
	readErr := func() error {
		defer publisher.CloseAllSubscribers()
		// Some subscribers may need specfic information before being able to perform their tasks. For example, (CreateInterpolatedMatlabFile)
		// Because of this, they will need their first message to set paramaters. This is what initMessage is for.
		initMessage := make(map[string]interface{})
		initMessage["file_name"] = matFileName
		initMessage["file_path"] = job.FileDir
		publisher.Publish(ctx, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage})
		no_schema_channels := make(map[string]bool)

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				publisher.Publish(ctx, &utils.DecodedMessage{Topic: messaging.EOF, Data: initMessage})
				break
			}

			if err != nil {
				return fmt.Errorf("error reading mcap message: %w", err)
			}

			if schema == nil {
				_, is_repeat_offender := no_schema_channels[channel.Topic]
				if !is_repeat_offender {
					no_schema_channels[channel.Topic] = true
					log.Printf("no schema found for channel ID: %d, channel: %v", message.ChannelID, channel)
				}
				continue
			}

			decodedMessage, err := mcapUtils.GetDecodedMessage(schema, message)
			if err != nil {
				continue
			}

			publisher.Publish(ctx, decodedMessage)
		}

		return nil
	}()

	publisher.WaitForClosure()

	log.Printf("All subscribers finished for job %v", job.ID)

	if readErr != nil {
		return nil, readErr
	}

	return publisher.Results(), nil
}

func routeMCAPDecodedMessage(ctx context.Context, decodedMessage *utils.DecodedMessage, possibleRoutes []string) []string {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, possibleRoutes...)
	case "hytech_msgs.VNData":
		subscriberNames = append(subscriberNames, messaging.LATLON, messaging.MAT)
	case "hytech_msgs.VehicleData":
		subscriberNames = append(subscriberNames, messaging.VELOCITY, messaging.MAT)
	default:
		subscriberNames = append(subscriberNames, messaging.MAT)
	}

	return subscriberNames
}
