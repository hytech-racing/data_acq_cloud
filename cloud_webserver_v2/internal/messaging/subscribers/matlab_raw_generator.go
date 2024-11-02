package subscribers

import (
	"strings"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"github.com/jhump/protoreflect/dynamic"
)

type RawMatlabWriter struct {
	allSignalData map[string]map[string]interface{}
	firstTime     *float64
}

func CreateRawMatlabWriter() *RawMatlabWriter {
	return &RawMatlabWriter{
		allSignalData: make(map[string]map[string]interface{}),
		firstTime:     nil,
	}
}

func (w *RawMatlabWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) error {
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
		w.processSignalValue(trimmedTopic, signalName, value, float64(decodedMessage.LogTime)/1e9)
	}

	return nil
}

func (w *RawMatlabWriter) processSignalValue(topic, signalName string, value interface{}, logTime float64) {
	dynamicMessage, ok := value.(*dynamic.Message)

	if ok {
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		// Process nested dynamic message fields
		currentNest := w.allSignalData[topic][signalName]
		w.addNestedValues(currentNest.(map[string]interface{}), dynamicMessage, logTime)

	} else {
		// Non-dynamic message values are processed normally
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make([][]float64, 0)
		}

		w.allSignalData[topic][signalName] = append(w.allSignalData[topic][signalName].([][]float64), []float64{logTime - *w.firstTime, utils.GetFloatValueOfInterface(value)})
	}
}

// Function to add nested values from dynamic message fields recursively
func (w *RawMatlabWriter) addNestedValues(nestedMap map[string]interface{}, dynamicMessage *dynamic.Message, logTime float64) {
	if dynamicMessage == nil {
		return
	}
	fieldNames := dynamicMessage.GetKnownFields()
	// Get all the field descriptors associated with this message
	for _, field := range fieldNames {
		fieldName := field.GetName()

		// Each dynamic message has field descriptors, not data. We need to extract those field descriptors and then use them
		// to figure out what data values are in there. The value could be another map, a list of values, or just a single value.
		// NOTE: We don't need to figure out what descriptors there are because we already decoded these messages in the GetDecodedMessage logic,
		// so they are just know associated with these dynamic.Message's.
		fieldDescriptor := dynamicMessage.FindFieldDescriptorByName(fieldName)
		if fieldDescriptor == nil {
			continue
		}

		decodedValue := dynamicMessage.GetField(fieldDescriptor)
		if decodedValue == nil {
			continue
		}

		unboxedNested, isNestedMessage := decodedValue.(*dynamic.Message)
		if isNestedMessage {
			// If it is a nested message and another map doesn't exist for it, we will make one to use.
			if _, ok := nestedMap[fieldName]; !ok {
				nestedMap[fieldName] = make(map[string]interface{})
			}

			w.addNestedValues(nestedMap[fieldName].(map[string]interface{}), unboxedNested, logTime)
		} else {
			if _, ok := nestedMap[fieldName]; !ok {
				nestedMap[fieldName] = make([][]float64, 0)
			}

			nestedMapFieldLength := len(nestedMap[fieldName].([][]float64))

			// Samples at 200hz
			if nestedMapFieldLength == 0 || nestedMap[fieldName].([][]float64)[nestedMapFieldLength-1][0]+0.005 <= (logTime-*w.firstTime) {
				nestedMap[fieldName] = append(nestedMap[fieldName].([][]float64), []float64{logTime - *w.firstTime, utils.GetFloatValueOfInterface(decodedValue)})
			}
		}
	}
}

func (w *RawMatlabWriter) AllSignalData() map[string]map[string]interface{} {
	return w.allSignalData
}

// func (w *RawMatlabWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) {
// 	if w.allSignalData[decodedMessage.Topic] == nil {
// 		w.allSignalData[decodedMessage.Topic] = make(map[string][][]interface{})
// 	}
//
// 	signalValues := decodedMessage.Data
//
// 	if w.firstTime == nil {
// 		firstValue := float64(decodedMessage.LogTime) / 1e9
// 		w.firstTime = &firstValue
// 	}
//
// 	if w.allSignalData[decodedMessage.Topic] == nil {
// 		w.allSignalData[decodedMessage.Topic] = make(map[string][][]interface{})
// 	}
//
// 	for signalName, value := range signalValues {
// 		// We call this method even though we are converting back to interfaces because some data types like bools should have specific values, and we need to account for that
// 		floatValue := utils.GetFloatValueOfInterface(value)
// 		signalSlice := w.allSignalData[decodedMessage.Topic][signalName] // All signal data for one signal value.
//
// 		timeSec := (float64(decodedMessage.LogTime) / 1e9) - *w.firstTime
// 		singleRowToAdd := []interface{}{timeSec, floatValue}
//
// 		signalSlice = append(signalSlice, singleRowToAdd)
// 		w.allSignalData[decodedMessage.Topic][signalName] = signalSlice
// 	}
// }
//
// func (w *RawMatlabWriter) GetAllSignalData() map[string]map[string][][]interface{} {
// 	return w.allSignalData
// }
