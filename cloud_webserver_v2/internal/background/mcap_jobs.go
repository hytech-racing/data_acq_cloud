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
	HDF5Location       string
	HDF5ObjectPath     string
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

// Process reads MCAPs and sends the messages to multiple subscribers which
// handle operations like creating HDF5 files and generating graphs.
// It also saves all this information to the database and stores files on S3.
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
	// files (the HDF5 file and its plots) are attached later, but only when the conversion
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
		removeGeneratedHDF5(job)
		return conversionErr
	}

	genericFileName := strings.Split(job.Filename, ".")[0]
	mcapResults, err := p.readMCAPMessages(ctx, job, job.ID)
	if err != nil {
		return fail(err)
	}

	// Extracting HDF5 file location from results
	var hdf5Location string
	if outer, ok := mcapResults[messaging.HDF5]; ok {
		if data, ok := outer.ResultData["file_path"]; ok {
			hdf5Location = data.(string)
		}
	}

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

	// Uploading HDF5 file to S3
	hdf5File, err := os.Open(hdf5Location)
	if err != nil {
		return fail(fmt.Errorf("could not open generated hdf5 file %v: %w", hdf5Location, err))
	}
	defer hdf5File.Close()

	hdf5FileName := fmt.Sprintf("%s.h5", genericFileName)
	matObjectFilePath := fmt.Sprintf("%s/%s", recordId.Hex(), hdf5FileName)
	if err := fp.s3Repository.PutObject(ctx, hdf5File, matObjectFilePath); err != nil {
		return fail(fmt.Errorf("failed to upload hdf5 file %v to s3: %w", hdf5FileName, err))
	}
	log.Printf("uploaded hdf5 file %v to s3", hdf5FileName)

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
		HDF5Location:       hdf5Location,
		HDF5ObjectPath:     matObjectFilePath,
		LatLonObjectPath:   vnLatLonPlotFileObjectPath,
		VelocityObjectPath: vnTimeVelPlotFileObjectPath,
	}

	if err := archiveToVolume(job, outputs); err != nil {
		if isRerun {
			removeGeneratedHDF5(job)
		}
		return err
	}

	if err := os.Remove(hdf5Location); err != nil {
		return fmt.Errorf("failed to remove created HDF5 file. McapFile: %w", err)
	}

	if isRerun {
		// Reload the run so metadata edits made while the conversion was running survive.
		vehicleRunModel, err = fp.dbClient.VehicleRunUseCase().GetVehicleRunById(ctx, recordId)
		if err != nil {
			return fmt.Errorf("could not load vehicle run %v after re-conversion: %w", recordId.Hex(), err)
		}
	}

	bucket := fp.s3Repository.Bucket()
	vehicleRunModel.MatFiles = []models.FileModel{convertedFile(bucket, outputs.HDF5ObjectPath)}
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

// removeGeneratedHDF5 deletes the local HDF5 file a conversion may have created.
func removeGeneratedHDF5(job *FileJob) {
	generatedHdf5Location := fmt.Sprintf("%s/%s.h5", job.FileDir, job.ID)
	if err := os.Remove(generatedHdf5Location); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to remove generated hdf5 file %v: %v", generatedHdf5Location, err)
	}
}

func convertedFile(bucket, objectPath string) models.FileModel {
	return models.FileModel{
		AwsBucket: bucket,
		FilePath:  objectPath,
		FileName:  path.Base(objectPath),
	}
}

// archiveToVolume copies the MCAP and its generated HDF5 file into the production
// metadata volume so other services can read them without going through S3.
func archiveToVolume(job *FileJob, outputs *mcapConversionOutputs) error {
	if os.Getenv("ENV") != "PRODUCTION" {
		return nil
	}

	// Directly uploaded MCAPs keep the S3 key the client wrote to, which has more
	// path segments than the record ID used for generated files. Create the parent
	// of each destination rather than assuming one layout.
	hdf5VolumePath := filepath.Join(RUN_METADATA_DIR, outputs.HDF5ObjectPath)
	mcapVolumePath := filepath.Join(RUN_METADATA_DIR, job.SourceS3Key)
	if err := os.MkdirAll(filepath.Dir(hdf5VolumePath), 0o755); err != nil {
		return fmt.Errorf("failed to create the run metadata directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(mcapVolumePath), 0o755); err != nil {
		return fmt.Errorf("failed to create the run metadata directory: %w", err)
	}

	srcHdf5File, err := os.Open(outputs.HDF5Location)
	if err != nil {
		return fmt.Errorf("failed to open h5 file for the volume: %w", err)
	}
	defer srcHdf5File.Close()

	// Create the HDF5 file in the volume
	destHdf5File, err := os.Create(hdf5VolumePath)
	if err != nil {
		return fmt.Errorf("error to create h5 file in volume %w", err)
	}
	defer destHdf5File.Close()

	// Copy the HDF5 file contents over to the file in the volume
	if _, err := io.Copy(destHdf5File, srcHdf5File); err != nil {
		return fmt.Errorf("failed to copy h5 file over to volume: %w", err)
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
			log.Printf("failed to copy mcap file over to volume: %v", err)
		}
	}

	return nil
}

// readMCAPMessages reads an MCAP file and routes the topics to subscribers to perform operations on it.
// By default, we create a vectornav latitude and longitude plot and an HDF5 file with data sampled at 200hz.
// It collects all the results (map[string]SubscriberResult aliased by SubscriberResults) generated by the subscribers
// and returns that.
func (p *PostProcessMCAPUploadJob) readMCAPMessages(ctx context.Context, job *FileJob, hdf5FileName string) (messaging.SubscriberResults, error) {
	// This is all the subsribers relavent to handling an MCAP mcapFile. You can attach more workers here if need be.
	subscriberMapping := make(map[string]messaging.SubscriberFunc)
	subscriberMapping[messaging.LATLON] = messaging.PlotLatLon
	subscriberMapping[messaging.VELOCITY] = messaging.PlotTimeVelocity
	subscriberMapping[messaging.HDF5] = messaging.CreateRawHDF5File

	return p.publishMCAPMessages(ctx, job, hdf5FileName, subscriberMapping)
}

// conversionMu serializes conversions because gonum's HDF5 binding links a C library
// built without thread safety. One conversion at a time also bounds its CPU and memory.
var conversionMu sync.Mutex

// publishMCAPMessages decodes every message in the job's MCAP file and publishes it to the given subscribers.
// The whole subscriber run is serialized, including the HDF5 file creation and close inside its subscriber.
func (p *PostProcessMCAPUploadJob) publishMCAPMessages(
	ctx context.Context,
	job *FileJob,
	hdf5FileName string,
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
		initMessage["schema_list"] = mcapReader.SchemaList
		initMessage["file_name"] = hdf5FileName
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
		subscriberNames = append(subscriberNames, messaging.LATLON, messaging.HDF5)
	case "hytech_msgs.VehicleData":
		subscriberNames = append(subscriberNames, messaging.VELOCITY, messaging.HDF5)
	default:
		subscriberNames = append(subscriberNames, messaging.HDF5)
	}

	return subscriberNames
}
