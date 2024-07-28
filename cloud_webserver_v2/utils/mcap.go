package utils

import (
	"fmt"
	"io"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/jhump/protoreflect/dynamic"
)

type mcapUtils struct {
	pbUtils *protobufUtils
}

type DecodedMessage struct {
	Topic string
	Data  map[string]interface{}
}

func NewMcapUtils() *mcapUtils {
	return &mcapUtils{
		pbUtils: NewProtobufUtils(),
	}
}

func (m *mcapUtils) NewReader(r io.Reader) (*mcap.Reader, error) {
	reader, err := mcap.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to build reader: %w", err)
	}
	return reader, nil
}

func (m *mcapUtils) GetDecodedMessage(schema *mcap.Schema, message *mcap.Message) (DecodedMessage, error) {
	decodedSchema, err := m.pbUtils.GetDecodedSchema(schema)
	if err != nil {
		fmt.Errorf("Failed to load schema")
	}

	messageDescriptor := decodedSchema.FindMessage(schema.Name)
	if messageDescriptor == nil {
		fmt.Errorf("Failed to find descriptor after loading pool")
	}

	dynMsg := dynamic.NewMessage(messageDescriptor)
	if err := dynMsg.Unmarshal(message.Data); err != nil {
		fmt.Errorf("Failed to parse message using included schema: %v", err)
	}

	decodedMessage := DecodedMessage{
		Topic: schema.Name,
		Data:  make(map[string]interface{}),
	}
	fields := dynMsg.GetKnownFields()
	for _, field := range fields {
		value := dynMsg.GetField(field)
		decodedMessage.Data[field.GetName()] = value
	}
	return decodedMessage, nil
}
