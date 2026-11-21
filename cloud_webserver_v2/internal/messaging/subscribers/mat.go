package subscribers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/hytech-racing/cloud-webserver-v2/internal/matfile"
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"github.com/jhump/protoreflect/dynamic"
)

// RawMatWriter collects decoded MCAP messages into MATLAB signal columns and
// periodically writes them to a v7.3 MAT file through matfile.Writer.
type RawMatWriter struct {
	firstTime       *float64
	MatWriter       *matfile.Writer
	allSignalData   map[string]map[string]interface{}
	filePath        string
	maxSignalLength int

	// ignoredSignals holds signals whose value type cannot be stored in a MAT
	// file, so they are logged once instead of once per message.
	ignoredSignals map[string]struct{}
}

func CreateRawMatWriter(filePath, fileName string) (*RawMatWriter, error) {
	matLocation := fmt.Sprintf("%s/%s.mat", filePath, fileName)
	log.Println(matLocation)
	matWriter, err := matfile.NewWriter(matLocation)
	if err != nil {
		return nil, err
	}

	return &RawMatWriter{
		allSignalData:   make(map[string]map[string]interface{}),
		firstTime:       nil,
		MatWriter:       matWriter,
		maxSignalLength: 0,
		filePath:        matLocation,
		ignoredSignals:  make(map[string]struct{}),
	}, nil
}

func (w *RawMatWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) error {
	if decodedMessage == nil || decodedMessage.Data == nil {
		return nil
	}
	signalValues := decodedMessage.Data

	// Some topics could be in the format of "hytech_msgs.MCUOutputData", but it's cleaner to just have MCUOutputData
	trimmedTopicSlice := strings.Split(decodedMessage.Topic, ".")
	trimmedTopic := trimmedTopicSlice[len(trimmedTopicSlice)-1]

	if w.firstTime == nil {
		firstValue := float64(decodedMessage.LogTime) / 1e9
		w.firstTime = &firstValue
	}

	if w.allSignalData[trimmedTopic] == nil {
		w.allSignalData[trimmedTopic] = make(map[string]interface{})
	}

	for signalName, value := range signalValues {
		w.processSignalValue(trimmedTopic, signalName, trimmedTopic+"."+signalName, value, float64(decodedMessage.LogTime)/1e9)
	}

	if w.maxSignalLength > 100_000 {
		err := w.MatWriter.WriteChunk(w.allSignalData)
		// Drop the written data no matter what: keeping a chunk that failed to write would
		// retry it for every later message, flooding the log and growing the map forever.
		w.allSignalData = make(map[string]map[string]interface{})
		w.maxSignalLength = 0
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *RawMatWriter) processSignalValue(topic, signalName, signalPath string, value interface{}, logTime float64) {
	switch value.(type) {
	case *dynamic.Message:
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		currentNest := w.allSignalData[topic][signalName]
		w.addNestedValues(signalPath, currentNest.(map[string]interface{}), value.(*dynamic.Message), logTime)
		return
	case map[string]interface{}:
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		currentNest := w.allSignalData[topic][signalName]
		w.addJSONValues(value.(map[string]interface{}), currentNest.(map[string]interface{}), logTime)
		return
	case []interface{}:
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		currentNest := w.allSignalData[topic][signalName]
		w.addNondynamicSliceValues(signalName, value, currentNest.(map[string]interface{}), logTime)
		return
	default:
		if !matfile.IsSupportedValue(value) {
			w.ignoreUnsupported(signalPath, value)
			return
		}
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = matfile.NewColumn(value)
		}

		column := w.allSignalData[topic][signalName].(*matfile.Column)
		column.Append(value, logTime-*w.firstTime)
		w.maxSignalLength = max(w.maxSignalLength, column.Len())
	}
}

// ignoreUnsupported records a signal whose values cannot be stored in a MAT file. It logs only
// the first occurrence so an unsupported type, such as a string or bytes field, does not flood
// the log once per message. A field's /Timestamp companion is expected to be dropped, so it is
// recorded without a log line.
func (w *RawMatWriter) ignoreUnsupported(signalName string, value interface{}) {
	if _, seen := w.ignoredSignals[signalName]; seen {
		return
	}
	if strings.HasSuffix(signalName, "/Timestamp") {
		w.ignoredSignals[signalName] = struct{}{}
		return
	}
	w.ignoredSignals[signalName] = struct{}{}
	log.Printf("ignoring signal %v in mat file: unsupported value type %T", signalName, value)
}

// appendDownsampled adds value to the signal column at givenMap[signalName],
// unless it is less than 5ms after the last value in that column.
func (w *RawMatWriter) appendDownsampled(givenMap map[string]interface{}, signalName string, value interface{}, logTime float64) {
	if !matfile.IsSupportedValue(value) {
		w.ignoreUnsupported(signalName, value)
		return
	}
	if _, ok := givenMap[signalName]; !ok {
		givenMap[signalName] = matfile.NewColumn(value)
	}
	column := givenMap[signalName].(*matfile.Column)

	if column.Len() == 0 || column.LastTimestamp()+0.005 <= (logTime-*w.firstTime) {
		column.Append(value, logTime-*w.firstTime)
		w.maxSignalLength = max(w.maxSignalLength, column.Len())
	}
}

// addJSONValues adds the scalar fields of one JSON object as signals.
func (w *RawMatWriter) addJSONValues(decodedValue map[string]interface{}, givenMap map[string]interface{}, logTime float64) {
	for signalName, value := range decodedValue {
		w.appendDownsampled(givenMap, signalName, value, logTime)
	}
}

// addNondynamicSliceValues adds each element of a repeated field as its own signal.
func (w *RawMatWriter) addNondynamicSliceValues(fieldName string, decodedValue interface{}, givenMap map[string]interface{}, logTime float64) {
	length := len(decodedValue.([]interface{}))
	for i := 0; i < length; i++ {
		repeatedFieldName := fieldName + "_" + strconv.Itoa(i)
		repeatedValue := decodedValue.([]interface{})[i]

		w.appendDownsampled(givenMap, repeatedFieldName, repeatedValue, logTime)
	}
}

// addNestedValues adds the scalar fields of a nested message as signals.
func (w *RawMatWriter) addNestedValues(signalPath string, nestedMap map[string]interface{}, dynamicMessage *dynamic.Message, logTime float64) {
	if dynamicMessage == nil {
		return
	}
	for _, field := range dynamicMessage.GetKnownFields() {
		fieldName := field.GetName()

		fieldDescriptor := dynamicMessage.FindFieldDescriptorByName(fieldName)
		if fieldDescriptor == nil {
			continue
		}

		decodedValue := dynamicMessage.GetField(fieldDescriptor)
		if decodedValue == nil {
			continue
		}

		if unboxedNested, isNestedMessage := decodedValue.(*dynamic.Message); isNestedMessage {
			if _, ok := nestedMap[fieldName]; !ok {
				nestedMap[fieldName] = make(map[string]interface{})
			}
			w.addNestedValues(signalPath+"."+fieldName, nestedMap[fieldName].(map[string]interface{}), unboxedNested, logTime)
			continue
		}

		if field.IsRepeated() {
			w.addNondynamicSliceValues(fieldName, decodedValue, nestedMap, logTime)
			continue
		}

		w.appendDownsampled(nestedMap, fieldName, decodedValue, logTime)
	}
}

func (w *RawMatWriter) AllSignalData() map[string]map[string]interface{} {
	return w.allSignalData
}

func (w *RawMatWriter) MaxSignalLength() int {
	return w.maxSignalLength
}

func (w *RawMatWriter) FilePath() string {
	return w.filePath
}
