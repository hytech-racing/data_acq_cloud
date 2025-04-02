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

// PostProcessMCAPUploadJob handles the post processing of MCAP files.
// PostProcessMCAPUploadJob serves as a wrapper struct to hold the Process function
// so it implicitely inherits FileJobProcessor.
type PostProcessMCAPUploadJob struct{}

// Process reads MCAPs and sends the messages to multiple subscribers which
// handle operations like creating HDF5 files and generating graphs.
// It also saves all this information to the database and stores files on S3.
func (p *PostProcessMCAPUploadJob) Process(fp *FileProcessor, job *FileJob) error {
	ctx := context.TODO()
	fp.setCurrentlyProcessing(true)
	fp.updateJobStatus(job, StatusProcessing)

	genericFileName := strings.Split(job.Filename, ".")[0]
	mcapResults, err := p.readMCAPMessages(ctx, job, genericFileName)
	if err != nil {
		return err
	}

	// Extracting HDF5 file location from results
	var hdf5Location string
	if outer, ok := mcapResults[messaging.MATLAB]; ok {
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

	// Uploading MCAP file to S3
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

	// Uploading HDF5 file to S3
	hdf5File, err := os.Open(hdf5Location)
	if err != nil {
		log.Fatalf("could not open mat matFile: %v", err)
	}
	defer hdf5File.Close()

	hdf5FileName := fmt.Sprintf("%s.h5", genericFileName)
	matObjectFilePath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, hdf5FileName)
	err = fp.s3Repository.WriteObjectReader(ctx, hdf5File, matObjectFilePath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded hdf5 file %v to s3", hdf5FileName)

	// Uploading Lat-Lon file to S3
	vnLatLonPlotName := fmt.Sprintf("%v_LatLon.png", genericFileName)
	vnLatLonPlotFileObjectPath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, vnLatLonPlotName)
	err = fp.s3Repository.WriteObjectWriterTo(ctx, vnLatLonPlotWriter, vnLatLonPlotFileObjectPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded vn lat lon plot %v to s3", vnLatLonPlotName)

	// Uploading Time-Vel file to S3
	vnTimeVelPlotName := fmt.Sprintf("%v_Velocity.png", genericFileName)
	vnTimeVelPlotFileObjectPath := fmt.Sprintf("%v-%v-%v/%s", month, day, year, vnTimeVelPlotName)
	err = fp.s3Repository.WriteObjectWriterTo(ctx, vnTimeVelPlotWriter, vnTimeVelPlotFileObjectPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded vn time vel plot %v to s3", vnTimeVelPlotName)

	// After successful processing, if we are in PRODUCTION, save the mcap and h5 file to our docker volume
	if os.Getenv("ENV") == "PRODUCTION" {
		// Create the directory structure for the files
		os.MkdirAll(fmt.Sprintf("/data/run_metadata/%v-%v-%v", month, day, year), os.ModeDir)

		// Create the HDF5 file in the volume
		destHdf5File, err := os.Create(fmt.Sprintf("/data/run_metadata/%s", matObjectFilePath))
		if err != nil {
			return fmt.Errorf("error to create h5 file in volume %w", err)
		}
		defer destHdf5File.Close()

		// Copy the HDF5 file contents over to the file in the volume
		_, err = io.Copy(destHdf5File, hdf5File)
		if err != nil {
			fmt.Printf("failed to copy h5 file over to volume: %w", err)
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
			fmt.Printf("failed to copy mcap file over to volume: %w", err)
		}
	}

	if err := os.Remove(hdf5Location); err != nil {
		return fmt.Errorf("failed to remove created mat mcapFile: %w", err)
	}

	if err := os.Remove(job.FilePath); err != nil {
		return fmt.Errorf("failed to remove processed mcapFile: %w", err)
	}

	// Create the models to upload into the database
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

	vnTimeVelPlotFileEntry := models.FileModel{
		AwsBucket: fp.s3Repository.Bucket(),
		FilePath:  vnTimeVelPlotFileObjectPath,
		FileName:  vnTimeVelPlotName,
	}
	vnTimeVelPlotFiles := []models.FileModel{vnTimeVelPlotFileEntry}
	contentFiles["vn_time_vel_plot"] = vnTimeVelPlotFiles

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

	// Update the file processor's total size and estimated size after removing
	fp.TotalSize.Add(-job.Size)
	fp.MiddlewareEstimatedSize.Add(-job.Size)
	fp.updateJobStatus(job, StatusCompleted)
	fp.setCurrentlyProcessing(false)

	log.Printf("Completed job %v", job.ID)
	return nil
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
	subscriberMapping[messaging.MATLAB] = messaging.CreateRawMatlabFile

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
		initMessage["schema_list"] = mcapReader.SchemaList
		initMessage["file_name"] = genericFileName
		initMessage["file_path"] = job.FileDir
		p.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage}, &subscriber_names)

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				p.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.EOF}, &subscriber_names)
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

			p.routeMessagesToSubscribers(ctx, publisher, decodedMessage, &subscriber_names)
		}

		// Need to make sure to close the subscribers or our code will hang and wait forever
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	log.Printf("All subscribers finished for job %v", job.ID)

	return publisher.Results(), nil
}

func (p *PostProcessMCAPUploadJob) routeMessagesToSubscribers(ctx context.Context, publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, *allNames...)
	case "hytech_msgs.VNData":
		subscriberNames = append(subscriberNames, messaging.LATLON, messaging.MATLAB)
	case "hytech_msgs.VehicleData":
		subscriberNames = append(subscriberNames, messaging.VELOCITY, messaging.MATLAB)
	default:
		subscriberNames = append(subscriberNames, messaging.MATLAB)
	}

	publisher.Publish(ctx, decodedMessage, subscriberNames)
}
