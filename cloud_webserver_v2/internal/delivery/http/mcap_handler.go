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
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

type mcapHandler struct{}

func NewMcapHandler(r *chi.Mux) {
	handler := &mcapHandler{}

	r.Route("/mcaps", func(r chi.Router) {
		r.Post("/upload", handler.UploadMcap)
	})
}

func (h *mcapHandler) UploadMcap(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(int64(math.Pow(10, 9)))
	if err != nil {
		fmt.Errorf("cloud not parse mutlipart form")
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Errorf("could not get the mcap file")
	}
	defer file.Close()

	fmt.Println("Uploaded file: %+v", handler.Filename)
	fmt.Println("File size: %+v", handler.Size)
	fmt.Println("MIME Header: %+v", handler.Header)

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

	publisher := messaging.NewPublisher()
	subscribers := []messaging.SubscriberFunc{
		messaging.PrintMessages,
	}
	for i, sub := range subscribers {
		name := fmt.Sprintf("Subscriber%d", i+1)
		publisher.Subscribe(i, name, sub)
	}

	go func() {
		for {
			schema, channel, message, err := message_iterator.NextInto(nil)
			if errors.Is(err, io.EOF) {
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

			publisher.Publish(&decodedMessage, []string{"Subscriber1"})
		}
	}()
	publisher.CloseAllSubscribers()
	fmt.Println("All Subscribers finished")
}
