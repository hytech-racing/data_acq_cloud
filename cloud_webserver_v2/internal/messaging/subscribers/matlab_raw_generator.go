package subscribers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
	"github.com/jhump/protoreflect/dynamic"
)

// This constructs a HDF5 file with a stream of messages it gets from a MCAP file.
// It chunk writes to the HDF5 file in groups.
// It does this by saving information into allSignalData and occasionally chunk writes all the data into the HDF5 file.
type RawMatlabWriter struct {
	firstTime       *float64
	HDF5Writer      *utils.HDF5Writer
	allSignalData   map[string]map[string]interface{}
	filePath        string
	failedMessages  [][2]interface{}
	maxSignalLength int // Constantly updated so we know what the max len of a data slice is
}

func CreateRawMatlabWriter(filePath, fileName string) (*RawMatlabWriter, error) {
	hdf5Location := fmt.Sprintf("%s/%s.h5", filePath, fileName)
	log.Println(hdf5Location)
	hdf5Writer, err := utils.NewHDF5Writer(hdf5Location)
	if err != nil {
		return nil, err
	}

	return &RawMatlabWriter{
		allSignalData:   make(map[string]map[string]interface{}),
		firstTime:       nil,
		failedMessages:  make([][2]interface{}, 0),
		HDF5Writer:      hdf5Writer,
		maxSignalLength: 0,
		filePath:        hdf5Location,
	}, nil
}

// AddSignalValue adds the values of the decodedMessage to allSignalData.
// If there exists a slice of signal values in allSignalData whose length is greater than
// maxSignalLength, then AddSignalValue will chunk write all the data in allSignalData to the
// currently open HDF5 file.
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
		w.processSignalValue(trimmedTopic, signalName, trimmedTopic+"."+signalName, value, float64(decodedMessage.LogTime)/1e9)
	}

	if w.maxSignalLength > 100_000 {
		err := w.HDF5Writer.ChunkWrite(w.allSignalData)
		if err != nil {
			return err
		}
		w.allSignalData = make(map[string]map[string]interface{})
		w.maxSignalLength = 0
	}

	return nil
}

// processSignalValue handles logic for whether to continue to dynamically decode the protobuf value or to directly add it to allSignalData
func (w *RawMatlabWriter) processSignalValue(topic, signalName, signalPath string, value interface{}, logTime float64) {
	switch value.(type) {
	case *dynamic.Message: // Dynamic message
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		// Process nested dynamic message fields
		currentNest := w.allSignalData[topic][signalName]
		w.addNestedValues(signalPath, currentNest.(map[string]interface{}), value.(*dynamic.Message), logTime)
		return
	case map[string]interface{}: // JSON message
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		currentNest := w.allSignalData[topic][signalName]
		w.addJSONValues(value.(map[string]interface{}), currentNest.(map[string]interface{}), signalPath, logTime)
		return
	case []interface{}: // Non-dynamic repeated message
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make(map[string]interface{})
		}

		currentNest := w.allSignalData[topic][signalName]
		w.addNondynamicSliceValues(signalName, value, currentNest.(map[string]interface{}), signalPath, logTime)
		return
	default: // Non-dynamic message
		if w.allSignalData[topic][signalName] == nil {
			w.allSignalData[topic][signalName] = make([]*utils.HDF5WrapperMessage, 0)
		}

		valueRawM := &utils.HDF5WrapperMessage{
			Data:      value,
			Timestamp: (logTime - *w.firstTime),
		}
		w.allSignalData[topic][signalName] = append(w.allSignalData[topic][signalName].([]*utils.HDF5WrapperMessage), valueRawM)
		w.maxSignalLength = max(w.maxSignalLength, len(w.allSignalData[topic][signalName].([]*utils.HDF5WrapperMessage)))
	}
}

// Func addJSONValues takes in decodedValue as a map and adds the values in the givenMap
// JSON messages ALWAYS come in maps of map -- example down below:
// &{map[ConfigureableTest:map[test_bool:false test_double:2 test_float:1 test_int:2 test_string:asdf]] drivebrain_configuration 1741122430108270412}
func (w *RawMatlabWriter) addJSONValues(decodedValue map[string]interface{}, givenMap map[string]interface{}, signalPath string, logTime float64) {
	baseSignalPath := signalPath
	for signalName, value := range decodedValue {
		baseSignalPath += "." + signalName
		if _, ok := givenMap[signalName]; !ok {
			givenMap[signalName] = make([]*utils.HDF5WrapperMessage, 0)
		}
		givenMapFieldLength := len(givenMap[signalName].([]*utils.HDF5WrapperMessage))

		if givenMapFieldLength == 0 || givenMap[signalName].([]*utils.HDF5WrapperMessage)[givenMapFieldLength-1].Timestamp+0.005 <= (logTime-*w.firstTime) {
			valueRawM := &utils.HDF5WrapperMessage{
				Data:      value,
				Timestamp: (logTime - *w.firstTime),
			}
			givenMap[signalName] = append(givenMap[signalName].([]*utils.HDF5WrapperMessage), valueRawM)
			w.maxSignalLength = max(w.maxSignalLength, len(givenMap[signalName].([]*utils.HDF5WrapperMessage)))
		}
		baseSignalPath = signalPath
	}
}

// addDynamicSliceValues process signals of Dynamic repeated/slice value
func (w *RawMatlabWriter) addDynamicSliceValues(fieldName string, decodedValue interface{}, givenMap map[string]interface{}, signalPath string, logTime float64) {
	baseSignalPath := signalPath
	unboxedNestedArr, _ := decodedValue.([]*dynamic.Message)
	for i, unboxedNested := range unboxedNestedArr {
		repeatedFieldName := fieldName + "_" + strconv.Itoa(i)
		baseSignalPath += "." + repeatedFieldName //

		if _, ok := givenMap[repeatedFieldName]; !ok {
			givenMap[repeatedFieldName] = make(map[string]interface{})
		}

		w.addNestedValues(signalPath, givenMap[repeatedFieldName].(map[string]interface{}), unboxedNested, logTime)
		baseSignalPath = signalPath
	}
}

// addNondynamicSliceValues process signals of Nondynamic repeated/slice value
func (w *RawMatlabWriter) addNondynamicSliceValues(fieldName string, decodedValue interface{}, givenMap map[string]interface{}, signalPath string, logTime float64) {
	baseSignalPath := signalPath

	length := len(decodedValue.([]interface{}))
	for i := 0; i < length; i++ {
		repeatedFieldName := fieldName + "_" + strconv.Itoa(i)
		baseSignalPath += "." + repeatedFieldName
		repeatedValue := decodedValue.([]interface{})[i]

		if _, ok := givenMap[repeatedFieldName]; !ok {
			givenMap[repeatedFieldName] = make([]*utils.HDF5WrapperMessage, 0)
		}
		givenMapFieldLength := len(givenMap[repeatedFieldName].([]*utils.HDF5WrapperMessage))

		if givenMapFieldLength == 0 || givenMap[repeatedFieldName].([]*utils.HDF5WrapperMessage)[givenMapFieldLength-1].Timestamp+0.005 <= (logTime-*w.firstTime) {
			valueRawM := &utils.HDF5WrapperMessage{
				Data:      repeatedValue,
				Timestamp: (logTime - *w.firstTime),
			}
			givenMap[repeatedFieldName] = append(givenMap[repeatedFieldName].([]*utils.HDF5WrapperMessage), valueRawM)

			w.maxSignalLength = max(w.maxSignalLength, len(givenMap[repeatedFieldName].([]*utils.HDF5WrapperMessage)))
		}
	}
}

// Function to add nested values from dynamic message fields recursively
func (w *RawMatlabWriter) addNestedValues(signalPath string, nestedMap map[string]interface{}, dynamicMessage *dynamic.Message, logTime float64) {
	if dynamicMessage == nil {
		return
	}
	fieldNames := dynamicMessage.GetKnownFields()
	// Get all the field descriptors associated with this message
	baseSignalPath := signalPath
	for _, field := range fieldNames {
		fieldName := field.GetName()

		baseSignalPath += "." + fieldName

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
			if _, ok := nestedMap[fieldName]; !ok {
				nestedMap[fieldName] = make(map[string]interface{})
			}

			// Dynamic repeated/slice message case
			isRepeated := field.IsRepeated() // if a field has repeated label, then take it as a slice
			if isRepeated {
				w.addDynamicSliceValues(fieldName, decodedValue, nestedMap, baseSignalPath, logTime)
				return
			}

			// Dynamic message case
			w.addNestedValues(baseSignalPath, nestedMap[fieldName].(map[string]interface{}), unboxedNested, logTime)
			return
		}

		// Non-dynamic repeated/slice message case
		isRepeated := field.IsRepeated()
		if isRepeated {
			if _, ok := nestedMap[fieldName]; !ok {
				nestedMap[fieldName] = make(map[string]interface{})
			}

			w.addNondynamicSliceValues(fieldName, decodedValue, nestedMap, baseSignalPath, logTime)
			continue
		}

		// ENUM message case
		enum := field.GetEnumType()
		if enum != nil {
			enumValue := enum.FindValueByNumber(decodedValue.(int32)) // Enum values always represented as int
			decodedValue = enumValue.GetName()                        // Just replace  decoded value with the its enum name value (not number value)
		}

		// Non-Dynamic message case
		if _, ok := nestedMap[fieldName]; !ok {
			nestedMap[fieldName] = make([]*utils.HDF5WrapperMessage, 0)
		}
		nestedMapFieldLength := len(nestedMap[fieldName].([]*utils.HDF5WrapperMessage))

		if nestedMapFieldLength == 0 || nestedMap[fieldName].([]*utils.HDF5WrapperMessage)[nestedMapFieldLength-1].Timestamp+0.005 <= (logTime-*w.firstTime) {
			valueRawM := &utils.HDF5WrapperMessage{
				Data:      decodedValue,
				Timestamp: (logTime - *w.firstTime),
			}
			nestedMap[fieldName] = append(nestedMap[fieldName].([]*utils.HDF5WrapperMessage), valueRawM)
			w.maxSignalLength = max(w.maxSignalLength, len(nestedMap[fieldName].([]*utils.HDF5WrapperMessage)))
		}

		baseSignalPath = signalPath
	}
}

func (w *RawMatlabWriter) AllSignalData() map[string]map[string]interface{} {
	return w.allSignalData
}

func (w *RawMatlabWriter) MaxSignalLength() int {
	return w.maxSignalLength
}

func (w *RawMatlabWriter) FilePath() string {
	return w.filePath
}
