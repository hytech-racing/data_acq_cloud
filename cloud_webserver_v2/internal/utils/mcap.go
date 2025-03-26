package utils

import (
	"encoding/json"
	"errors"
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

// McapReader contains information relevant to reading and parsing an MCAP file
type McapReader struct {
	Reader     *mcap.Reader
	Info       *mcap.Info
	SchemaList []string
}

// DecodedMessage contains decoded data from a protobuf encoded message
type DecodedMessage struct {
	Data    map[string]interface{}
	Topic   string
	LogTime uint64
}

func NewMcapUtils() *McapUtils {
	return &McapUtils{
		pbUtils: NewProtobufUtils(),
	}
}

func (m *McapUtils) NewReader(r io.Reader) (*McapReader, error) {
	reader, err := mcap.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to build reader: %w", err)
	}

	info, err := reader.Info()
	if err != nil {
		return nil, fmt.Errorf("could not get info for mcap reader: %v", err)
	}

	err = m.LoadAllSchemas(info)
	if err != nil {
		return nil, err
	}

	schemaList, err := m.GetMcapSchemaList(reader)
	if err != nil {
		return nil, err
	}

	return &McapReader{
		Reader:     reader,
		Info:       info,
		SchemaList: schemaList,
	}, nil
}

// GetDecodedMessage checks whether the encoding is json or protobuf and decodes
func (m *McapUtils) GetDecodedMessage(schema *mcap.Schema, message *mcap.Message) (*DecodedMessage, error) {
	// Schema encoding may only be omitted for self-describing message encodings such as json.
	if schema.Encoding == "json" || schema.Encoding == "jsonschema" || schema.Encoding == "" {
		decodedMessage, err := m.DecodeJSON(schema, message)
		if err != nil {
			return nil, err
		}
		return decodedMessage, nil
	} else if schema.Encoding == "protobuf" {
		decodedMessage, err := m.DecodeProtobuf(schema, message)
		if err != nil {
			return nil, err
		}
		return decodedMessage, nil
	}

	return nil, errors.New("message is not in protobuf or json format")
}

// DecodeJSON func takes in schema and messsage and decodes accordingly (think of JSON as Maps)
func (m *McapUtils) DecodeJSON(schema *mcap.Schema, message *mcap.Message) (*DecodedMessage, error) {
	decodedMessage := DecodedMessage{
		Topic:   schema.Name,
		Data:    make(map[string]interface{}),
		LogTime: message.LogTime,
	}
	err := json.Unmarshal(message.Data, &decodedMessage.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON messsage: %v", err)
	}
	return &decodedMessage, nil
}

// DecodeProtobuf func takes in schema and messsage and decodes accordingly
func (m *McapUtils) DecodeProtobuf(schema *mcap.Schema, message *mcap.Message) (*DecodedMessage, error) {
	decodedSchema, err := m.pbUtils.GetDecodedSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to load protobuf schema: %v", err)
	}

	messageDescriptor := decodedSchema.FindMessage(schema.Name)
	if messageDescriptor == nil {
		return nil, errors.New("failed to find descriptor after loading pool")
	}

	dynMsg := dynamic.NewMessage(messageDescriptor)
	if err := dynMsg.Unmarshal(message.Data); err != nil {
		return nil, fmt.Errorf("failed to parse protobuf message using included schema: %v", err)
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

// LoadAllSchemas loads all the protobuf schemas found in the MCAP file into
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
	mcapSchemaMap := make(map[string]map[string][]float64)
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
