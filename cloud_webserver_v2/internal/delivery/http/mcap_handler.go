package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging"
	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

/* TODO for MCAP handler:
- [x] Add logic for parsing decoded MCAP files
- [x] Be able to send those messages out to subscribers
- [x] Be able to write MATLAB files from the MCAP inputs.
- [ ] Store/organize those MCAP and Matlab files in AWS S3 (waiting on py_data_acq to write MCAP files with dates/other info in metadata)
- [ ] After debugging, make UploadMcap route quickly give response and perform task after responding
- [ ] The interpolation logic is a little flawed. More docs on that is in the bookstack. We need to fix it but it is low-priority for now.
- [ ] Once interpolation logic is fixed, write an interpolated MCAP file with the data.
*/

type mcapHandler struct {
	s3_repository *s3.S3Repository
}

func NewMcapHandler(r *chi.Mux, s3_repository *s3.S3Repository) {
	handler := &mcapHandler{
		s3_repository: s3_repository,
	}

	r.Route("/mcaps", func(r chi.Router) {
		r.Post("/upload", handler.UploadMcap)
	})
}

/*
This route takes an MCAP file and performs a series of actions on it.
It currently:
  - plots the vectornav lat/lon onto a cartesian plane
  - creates an interpolated MATLAB file
  - creates a raw (no calculations performed onto it) MATLAB file
*/
func (h *mcapHandler) UploadMcap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Handles the file input from the HTTP Request
	err := r.ParseMultipartForm(int64(math.Pow(10, 9)))
	if err != nil {
		fmt.Errorf("cloud not parse mutlipart form")
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Errorf("could not get the mcap file")
	}
	defer file.Close()

	log.Printf("Uploaded file: %+v", handler.Filename)
	log.Printf("File size: %+v", handler.Size)
	log.Printf("MIME Header: %+v", handler.Header)

	mcapUtils := utils.NewMcapUtils()

	reader, err := mcapUtils.NewReader(file)
	if err != nil {
		fmt.Errorf("could not create mcap reader")
	}

	schemaList, err := mcapUtils.GetMcapSchemaList(reader)
	if err != nil {
		log.Panicf("%v", err)
	}

	message_iterator, err := reader.Messages()
	if err != nil {
		fmt.Errorf("could not get mcap mesages")
	}

	// This is all the subsribers relavent to this POST request. You can attach more workers here if need be.
	subscriberMapping := make(map[string]messaging.SubscriberFunc)
	subscriberMapping["print"] = messaging.PrintMessages
	subscriberMapping["vn_plot"] = messaging.PlotLatLon
	subscriberMapping["matlab_writer"] = messaging.CreateInterpolatedMatlabFile

	publisher := messaging.NewPublisher(true)
	subscriber_names := make([]string, len(subscriberMapping))
	idx := 0
	for subscriber_name, function := range subscriberMapping {
		subscriber_names[idx] = subscriber_name
		publisher.Subscribe(idx+1, subscriber_name, function)
		idx++
	}

	go func() {
		// Some subscribers may need specfic information before being able to perform their tasks. For example, (CreateInterpolatedMatlabFile)
		// Because of this, they will need their first message to set paramaters. This is what initMessage is for.
		initMessage := make(map[string]interface{})
		initMessage["schema_list"] = schemaList
		h.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.INIT, Data: initMessage}, &subscriber_names)

		for {
			schema, channel, message, err := message_iterator.NextInto(nil)

			// Checks if we have no more messages to read from the MCAP. If so, it lets the subscribers know
			if errors.Is(err, io.EOF) {
				h.routeMessagesToSubscribers(ctx, publisher, &utils.DecodedMessage{Topic: messaging.EOF}, &subscriber_names)
				break
			}

			if err != nil {
				log.Fatalf("error reading mcap message: %v", err)
			}

			if schema == nil {
				log.Printf("no schema found for channel ID: %d, channel: %v", message.ChannelID, channel)
				continue
			}

			decodedMessage, err := mcapUtils.GetDecodedMessage(schema, message)
			if err != nil {
				log.Printf("could not decode message: %v", err)
			}

			h.routeMessagesToSubscribers(ctx, publisher, &decodedMessage, &subscriber_names)
		}

		// Need to make sure to close the subscribers or our code will hang and wait forever
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	subscriberResults := publisher.GetResults()
	interpolatedData := subscriberResults["matlab_writer"].ResultData["interpolated_data"]
	utils.CreateMatlabFile(interpolatedData.(*map[string]map[string][]float64))

	// Logic to get all the misc. information

	//TODO:
	/*
		Upload files to the AWS s3_repository
		Compile all data to store in MongoDB database
		Cleanup
	*/

	fmt.Println("All Subscribers finished")
}

func (h *mcapHandler) routeMessagesToSubscribers(ctx context.Context, publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, *allNames...)
	case "vn_lat_lon":
		subscriberNames = append(subscriberNames, "vn_plot", "matlab_writer")
	default:
		subscriberNames = append(subscriberNames, "matlab_writer")
	}

	publisher.Publish(ctx, decodedMessage, subscriberNames)
}
