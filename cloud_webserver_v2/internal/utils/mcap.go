package utils

import (
	"fmt"
	"io"
	"strconv"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/jhump/protoreflect/dynamic"
)

type mcapUtils struct {
	pbUtils *protobufUtils
}

type DecodedMessage struct {
	Topic   string
	Data    map[string]interface{}
	LogTime uint64
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
		Topic:   schema.Name,
		Data:    make(map[string]interface{}),
		LogTime: message.LogTime,
	}

	fields := dynMsg.GetKnownFields()
	for _, field := range fields {
		value := dynMsg.GetField(field)
		decodedMessage.Data[field.GetName()] = value
	}
	return decodedMessage, nil
}

func (m *mcapUtils) GetMcapSchemaList(reader *mcap.Reader) ([]string, error) {
	mcapInfo, err := reader.Info()
	if err != nil {
		return nil, err
	}
	schemaList := make([]string, 0)

	for _, schema := range mcapInfo.Schemas {
		schemaList = append(schemaList, schema.Name)
	}

	return schemaList, nil
}

func GetMcapSchemaMap(schemaList []string) (map[string]map[string][]float64, error) {
	var mcapSchemaMap map[string]map[string][]float64

	mcapSchemaMap = make(map[string]map[string][]float64)
	mcapSchemaMap["global_times"] = make(map[string][]float64)
	mcapSchemaMap["global_times"]["times"] = make([]float64, 0)

	for _, schemaName := range schemaList {
		mcapSchemaMap[schemaName] = make(map[string][]float64)
	}

	return mcapSchemaMap, nil
}

func GetFloatValueOfInterface(val interface{}) float64 {
	var out float64

	switch x := val.(type) {
	case int32:
		out = float64(x)
	case uint64:
		out = float64(x)
	case float32:
		out = float64(x)
	case string:
		i, err := strconv.Atoi(x)
		if err != nil {
			panic(err)
		}
		out = float64(i)
	case bool:
		if val.(bool) {
			out = 1
		} else {
			out = 0
		}
	}

	return out
}
