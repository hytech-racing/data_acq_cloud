package handler

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hytech-racing/cloud-webserver-v2/utils"
	"github.com/jhump/protoreflect/dynamic"
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

	protobufUtils := utils.NewProtobufUtils()
	schema, channel, message, err := message_iterator.NextInto(nil)
	count := 0
	for !errors.Is(err, io.EOF) {

		fd, err := protobufUtils.GetDecodedSchema(schema)
		if err != nil {
			fmt.Errorf("Failed to load schema")
		}

		messageDescriptor := fd.FindMessage(schema.Name)
		if messageDescriptor == nil {
			fmt.Errorf("Failed to find descriptor after loading pool")
		}

		dynMsg := dynamic.NewMessage(messageDescriptor)
		if err := dynMsg.Unmarshal(message.Data); err != nil {
			fmt.Errorf("Failed to parse message using included schema: %v", err)
		}

		fields := dynMsg.GetKnownFields()
		fmt.Printf("%s\t(%s)\t[%d]:\t{ ", channel.Topic, schema.Name, message.LogTime)
		for _, field := range fields {
			value := dynMsg.GetField(field)
			fmt.Printf("%s ", field.GetName())
			fmt.Println("%s ", value)
		}
		fmt.Println("} \n\n\n")

		schema, _, message, err = message_iterator.NextInto(nil)
		count++
	}
	println(count)
}
