package utils

import (
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/jhump/protoreflect/dynamic"
)

type McapUtils struct {
	pbUtils *ProtobufUtils
}

type DecodedMessage struct {
	Topic   string
	Data    map[string]interface{}
	LogTime uint64
}

func NewMcapUtils() *McapUtils {
	return &McapUtils{
		pbUtils: NewProtobufUtils(),
	}
}

func (m *McapUtils) NewReader(r io.Reader) (*mcap.Reader, error) {
	reader, err := mcap.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to build reader: %w", err)
	}
	return reader, nil
}

func (m *McapUtils) GetDecodedMessage(schema *mcap.Schema, message *mcap.Message) (*DecodedMessage, error) {
	decodedSchema, err := m.pbUtils.GetDecodedSchema(schema)
	if err != nil {
		fmt.Errorf("Failed to load schema")
		return nil, err
	}

	messageDescriptor := decodedSchema.FindMessage(schema.Name)
	if messageDescriptor == nil {
		fmt.Errorf("Failed to find descriptor after loading pool")
		return nil, err
	}

	dynMsg := dynamic.NewMessage(messageDescriptor)
	if err := dynMsg.Unmarshal(message.Data); err != nil {
		return nil, fmt.Errorf("Failed to parse message using included schema: %v", err)
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
	return &decodedMessage, nil
}

func (m *McapUtils) LoadAllSchemas(info *mcap.Info) error {
	schemas := info.Schemas
	retrySchemas := make([]*mcap.Schema, 0)

	for _, schema := range schemas {
		retrySchemas = append(retrySchemas, schema)
	}

	maxRetries := len(retrySchemas) + 1
	for range maxRetries {
		if len(retrySchemas) == 0 {
			break
		}

		newRetries := make([]*mcap.Schema, 0)
		for _, schema := range retrySchemas {
			_, err := m.pbUtils.GetDecodedSchema(schema)
			if err != nil {
				newRetries = append(newRetries, schema)
			}
		}

		retrySchemas = newRetries
	}

	return nil
}

func (m *McapUtils) GetMcapSchemaList(reader *mcap.Reader) ([]string, error) {
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
	if val == nil {
		return 0
	}

	var out float64

	switch x := val.(type) {
	case float32:
		out = float64(x)
	case uint64:
		out = float64(x)
	case int32:
		out = float64(x)
	case string:
		if x == "" {
			return 0
		}
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

	if math.IsInf(out, 1) {
		out = math.MaxFloat32
	} else if math.IsInf(out, -1) {
		out = math.SmallestNonzeroFloat32
	} else if math.IsNaN(out) {
		out = math.MaxFloat32
	}

	return out
}
