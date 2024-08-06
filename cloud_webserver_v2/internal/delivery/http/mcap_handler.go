package http

import (
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

type mcapHandler struct {
	subscriber_mapping map[string]messaging.SubscriberFunc
	s3_repository      *s3.S3Repository
}

func NewMcapHandler(r *chi.Mux, s3_repository *s3.S3Repository) {
	subscriber_mapping := make(map[string]messaging.SubscriberFunc)
	subscriber_mapping["print"] = messaging.PrintMessages
	subscriber_mapping["vn_plot"] = messaging.PlotLatLon

	handler := &mcapHandler{
		subscriber_mapping: subscriber_mapping,
		s3_repository:      s3_repository,
	}

	r.Route("/mcaps", func(r chi.Router) {
		r.Post("/upload", handler.UploadMcap)
	})
}

func (h *mcapHandler) UploadMcap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := r.ParseMultipartForm(int64(math.Pow(10, 9)))
	if err != nil {
		fmt.Errorf("cloud not parse mutlipart form")
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Errorf("could not get the mcap file")
	}
	defer file.Close()

	log.Println("Uploaded file: %+v", handler.Filename)
	log.Println("File size: %+v", handler.Size)
	log.Println("MIME Header: %+v", handler.Header)

	mcapUtils := utils.NewMcapUtils()
	reader, err := mcapUtils.NewReader(file)
	fmt.Errorf("could not create mcap reader")
	if err != nil {
		fmt.Errorf("could not create mcap reader")
	}

	message_iterator, err := reader.Messages()
	if err != nil {
		fmt.Errorf("could not get mcap mesages")
	}

	publisher := messaging.NewPublisher(true)
	subscriber_names := make([]string, len(h.subscriber_mapping))
	idx := 0
	for subscriber_name, function := range h.subscriber_mapping {
		subscriber_names[idx] = subscriber_name
		publisher.Subscribe(idx+1, subscriber_name, function)
		idx++
	}

	go func() {
		// Required to call CollectResults if using channels which send responses. This is because it creates the channel which the
		// results will be sent through
		for {
			schema, channel, message, err := message_iterator.NextInto(nil)
			if errors.Is(err, io.EOF) {
				h.routeMessagesToSubscribers(publisher, &utils.DecodedMessage{Topic: messaging.EOF}, &subscriber_names)
				break
			}

			if err != nil {
				log.Fatalf("error reading mcap message: %v", err)
			}

			if schema == nil {
				log.Printf("no schema found for channel ID: %d, channel: %s", message.ChannelID, channel)
				continue
			}

			decodedMessage, err := mcapUtils.GetDecodedMessage(schema, message)
			if err != nil {
				log.Printf("could not decode message: %v", err)
			}

			h.routeMessagesToSubscribers(publisher, &decodedMessage, &subscriber_names)
		}
		publisher.CloseAllSubscribers()
	}()

	publisher.WaitForClosure()

	results := publisher.GetResults()
	for key, val := range results {
		if key == "vn_plot" {
			data := val.ResultData
			log.Printf("found data %v \n ", data)
			if writer_to, ok := data["writer_to"].(*io.WriterTo); ok {
				log.Printf("is it ok, %v", ok)

				log.Printf("found writer \n ")
				h.s3_repository.WriteObject(ctx, writer_to, "object")
			}
		}
	}

	fmt.Println("All Subscribers finished")
}

func (h *mcapHandler) routeMessagesToSubscribers(publisher *messaging.Publisher, decodedMessage *utils.DecodedMessage, allNames *[]string) {
	// List of all the workers we want to send the messages to
	var subscriberNames []string
	switch topic := decodedMessage.Topic; topic {
	case messaging.EOF:
		subscriberNames = append(subscriberNames, *allNames...)
	case "vn_lat_lon":
		subscriberNames = append(subscriberNames, "matlab", "vn_plot")
	default:
		subscriberNames = append(subscriberNames, "matlab")
	}

	publisher.Publish(decodedMessage, subscriberNames)
}
