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
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PostProcessMCAPUploadJob handles the post processing of MCAP files.
// PostProcessMCAPUploadJob serves as a wrapper struct to hold the Process function
// so it implicitely inherits FileJobProcessor.
type PostProcessMCAPUploadJob struct{}

// Process reads MCAPs and sends the messages to multiple subscribers which
// handle operations like creating HDF5 files and generating graphs.
// It also saves all this information to the database and stores files on S3.
func (p *PostProcessMCAPUploadJob) ProcessFileJob(fp *FileProcessor, job *FileJob) error {
	ctx := context.Background()
	fp.broadcastFileUploadStart(job)

	// Uploading MCAP file to S3 before any conversion is attempted. This guarantees the
	// original MCAP is preserved even if converting it into an HDF5 file fails.
	mcapFileS3Reader, err := os.Open(job.FilePath)
	if err != nil {
		return fmt.Errorf("could not open mcap file %v: %w", job.FilePath, err)
	}
	defer mcapFileS3Reader.Close()

	recordId := primitive.NewObjectID()
	mcapFileName := job.Filename
	mcapObjectFilePath := fmt.Sprintf("%s/%s", recordId.Hex(), mcapFileName)
	if err := fp.s3Repository.WriteObjectReader(ctx, mcapFileS3Reader, mcapObjectFilePath); err != nil {
		return fmt.Errorf("failed to upload mcap file %v to s3: %w", mcapFileName, err)
	}
	log.Printf("uploaded mcap file %v to s3", mcapFileName)

	// The file hash is created before any conversion happens so that runs which only ever
	// end up with an MCAP file can still be found through the /mcaps/status hash lookup.
	fileHash, err := utils.CreateFileHash(mcapFileS3Reader)
	if err != nil {
		return fmt.Errorf("failed to create file hash for %v: %w", mcapFileName, err)
	}

	// The vehicle run is created upfront with only the MCAP file attached to it. Converted
	// files (the HDF5 file and its plots) are attached later, but only when the conversion
	// succeeds. This makes sure an uploaded MCAP is always returned by the fetch endpoints.
	vehicleRunModel := &models.VehicleRunModel{
		Date:     job.Date,
		CarModel: "HT09",
		McapFiles: []models.FileModel{
			{
				AwsBucket: fp.s3Repository.Bucket(),
				FilePath:  mcapObjectFilePath,
				FileName:  mcapFileName,
				FileHash:  fileHash,
			},
		},
		Id: recordId,
	}

	genericFileName := strings.Split(job.Filename, ".")[0]
	mcapResults, err := p.readMCAPMessages(ctx, job, genericFileName)
	if err != nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, err)
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
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("could not open generated hdf5 file %v: %w", hdf5Location, err))
	}
	defer hdf5File.Close()

	hdf5FileName := fmt.Sprintf("%s.h5", genericFileName)
	matObjectFilePath := fmt.Sprintf("%s/%s", recordId.Hex(), hdf5FileName)
	if err := fp.s3Repository.WriteObjectReader(ctx, hdf5File, matObjectFilePath); err != nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("failed to upload hdf5 file %v to s3: %w", hdf5FileName, err))
	}
	log.Printf("uploaded hdf5 file %v to s3", hdf5FileName)

	// Uploading Lat-Lon file to S3
	if vnLatLonPlotWriter == nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("no lat-lon plot could be generated for %v", job.Filename))
	}
	vnLatLonPlotName := fmt.Sprintf("%v_LatLon.png", genericFileName)
	vnLatLonPlotFileObjectPath := fmt.Sprintf("%s/%s", recordId.Hex(), vnLatLonPlotName)
	if err := fp.s3Repository.WriteObjectWriterTo(ctx, vnLatLonPlotWriter, vnLatLonPlotFileObjectPath); err != nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("failed to upload vn lat lon plot %v to s3: %w", vnLatLonPlotName, err))
	}
	log.Printf("uploaded vn lat lon plot %v to s3", vnLatLonPlotName)

	// Uploading Time-Vel file to S3
	if vnTimeVelPlotWriter == nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("no time-velocity plot could be generated for %v", job.Filename))
	}
	vnTimeVelPlotName := fmt.Sprintf("%v_Velocity.png", genericFileName)
	vnTimeVelPlotFileObjectPath := fmt.Sprintf("%s/%s", recordId.Hex(), vnTimeVelPlotName)
	if err := fp.s3Repository.WriteObjectWriterTo(ctx, vnTimeVelPlotWriter, vnTimeVelPlotFileObjectPath); err != nil {
		return p.saveMcapOnlyVehicleRun(fp, ctx, job, vehicleRunModel, fmt.Errorf("failed to upload vn time vel plot %v to s3: %w", vnTimeVelPlotName, err))
	}
	log.Printf("uploaded vn time vel plot %v to s3", vnTimeVelPlotName)

	// After successful processing, if we are in PRODUCTION, save the mcap and h5 file to our docker volume
	if os.Getenv("ENV") == "PRODUCTION" {
		// Create the directory structure for the files
		os.MkdirAll(fmt.Sprintf("/data/run_metadata/%s", recordId.Hex()), os.ModeDir)

		// Create the HDF5 file in the volume
		destHdf5File, err := os.Create(fmt.Sprintf("/data/run_metadata/%s", matObjectFilePath))
		if err != nil {
			return fmt.Errorf("error to create h5 file in volume %w", err)
		}
		defer destHdf5File.Close()

		// Copy the HDF5 file contents over to the file in the volume
		_, err = io.Copy(destHdf5File, hdf5File)
		if err != nil {
			return fmt.Errorf("failed to copy h5 file over to volume: %w", err)
		}

		// Create the MCAP file in the volume
		destMcapFile, err := os.Create(fmt.Sprintf("/data/run_metadata/%s", mcapObjectFilePath))
		if err != nil {
			return fmt.Errorf("error to create mcap file in volume %w", err)
		}
		defer destMcapFile.Close()

		// Copy the MCAP file contents over to the file in the volume
		_, err = io.Copy(destMcapFile, mcapFileS3Reader)
		if err != nil {
			log.Printf("failed to copy mcap file over to volume: %v", err)
		}
	}

	if err := os.Remove(hdf5Location); err != nil {
		return fmt.Errorf("failed to remove created mat mcapFile: %w", err)
	}

	// Create the models to upload into the database
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

	vnTimeVelPlotFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  vnTimeVelPlotFileObjectPath,
		FileName:  vnTimeVelPlotName,
	}
	vnTimeVelPlotFiles := []models.FileModel{vnTimeVelPlotFileEntry}
	contentFiles["vn_time_vel_plot"] = vnTimeVelPlotFiles

	vehicleRunModel.MatFiles = matFiles
	vehicleRunModel.ContentFiles = contentFiles

	if _, err := fp.dbClient.VehicleRunUseCase().CreateVehicleRun(ctx, vehicleRunModel); err != nil {
		return fmt.Errorf("failed to save vehicle run for %v: %w", job.Filename, err)
	}

	// Cleanup the locally staged mcap file now that it has been uploaded to S3
	if err := os.Remove(job.FilePath); err != nil {
		log.Printf("failed to remove processed mcap file %v: %v", job.FilePath, err)
	}

	// Update the file processor's total size and estimated size after removing
	fp.TotalSize.Add(-job.Size)
	fp.MiddlewareEstimatedSize.Add(-job.Size)
	fp.broadcastFileUploadEnd(job, ctx, vehicleRunModel)

	log.Printf("Completed job %v", job.ID)
	return nil
}

func (p *PostProcessMCAPUploadJob) saveMcapOnlyVehicleRun(
	fp *FileProcessor, 
	ctx context.Context, 
	job *FileJob, 
	vehicleRunModel *models.VehicleRunModel, 
	conversionErr error,
) error {
	if _, err := fp.dbClient.VehicleRunUseCase().CreateVehicleRun(ctx, vehicleRunModel); err != nil {
		return fmt.Errorf("failed to save mcap-only vehicle run for %v: %w", job.Filename, err)
	}

	// Remove the partially generated HDF5 file, if one was created. Subscribers always
	// write it to "<file_dir>/<generic_file_name>.h5".
	generatedHdf5Location := fmt.Sprintf("%s/%s.h5", job.FileDir, strings.Split(job.Filename, ".")[0])
	if err := os.Remove(generatedHdf5Location); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to remove generated hdf5 file %v: %v", generatedHdf5Location, err)
	}

	// Cleanup the locally staged mcap file now that it is stored on S3
	if err := os.Remove(job.FilePath); err != nil {
		log.Printf("failed to remove processed mcap file %v: %v", job.FilePath, err)
	}

	// Update the file processor's total size and estimated size after removing
	fp.TotalSize.Add(-job.Size)
	fp.MiddlewareEstimatedSize.Add(-job.Size)

	log.Printf("saved vehicle run %v with only the mcap file: %v", vehicleRunModel.Id.Hex(), conversionErr)

	return conversionErr
}

// readMCAPMessages reads an MCAP file and routes the topics to subscribers to perform operations on it.
// By default, we create a vectornav latitude and longitude plot and an HDF5 file with data sampled at 200hz.
// It collects all the results (map[string]SubscriberResult aliased by SubscriberResults) generated by the subscribers
// and returns that.
func (p *PostProcessMCAPUploadJob) readMCAPMessages(ctx context.Context, job *FileJob, genericFileName string) (messaging.SubscriberResults, error) {
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

	// This is all the subsribers relavent to handling an MCAP mcapFile. You can attach more workers here if need be.
	subscriberMapping := make(map[string]messaging.SubscriberFunc)
	subscriberMapping[messaging.LATLON] = messaging.PlotLatLon
	subscriberMapping[messaging.VELOCITY] = messaging.PlotTimeVelocity
	subscriberMapping[messaging.HDF5] = messaging.CreateRawHDF5File

	publisher := messaging.NewPublisher().WithRouter(routeMCAPDecodedMessage).WithResultsListener()
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
		initMessage["schema_list"] = mcapReader.SchemaList
		initMessage["file_name"] = genericFileName
		initMessage["file_path"] = job.FileDir
		publisher.Publish(ctx, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage})

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				publisher.Publish(ctx, &utils.DecodedMessage{Topic: messaging.EOF, Data: initMessage})
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
				log.Printf("error decoding message: %v", err)
				continue
			}

			publisher.Publish(ctx, decodedMessage)
		}

		// Need to make sure to close the subscribers or our code will hang and wait forever
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	log.Printf("All subscribers finished for job %v", job.ID)

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
