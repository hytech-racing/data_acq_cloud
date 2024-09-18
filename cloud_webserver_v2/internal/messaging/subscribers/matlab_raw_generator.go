package subscribers

import "github.com/hytech-racing/cloud-webserver-v2/internal/utils"

type RawMatlabWriter struct {
	allSignalData map[string]map[string][][]interface{} // {"topic": {"signalValue": { [timeValue, signalValue] }}}
	firstTime     *float64
}

func CreateRawMatlabWriter() *RawMatlabWriter {
	return &RawMatlabWriter{
		allSignalData: make(map[string]map[string][][]interface{}),
		firstTime:     nil,
	}
}

func (w *RawMatlabWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) {
	if w.allSignalData[decodedMessage.Topic] == nil {
		w.allSignalData[decodedMessage.Topic] = make(map[string][][]interface{})
	}

	signalValues := decodedMessage.Data

	if w.firstTime == nil {
		firstValue := float64(decodedMessage.LogTime) / 1e9
		w.firstTime = &firstValue
	}

	if w.allSignalData[decodedMessage.Topic] == nil {
		w.allSignalData[decodedMessage.Topic] = make(map[string][][]interface{})
	}

	for signalName, value := range signalValues {
		// We call this method even though we are converting back to interfaces because some data types like bools should have specific values, and we need to account for that
		floatValue := utils.GetFloatValueOfInterface(value)
		signalSlice := w.allSignalData[decodedMessage.Topic][signalName] // All signal data for one signal value.

		timeSec := (float64(decodedMessage.LogTime) / 1e9) - *w.firstTime
		singleRowToAdd := []interface{}{timeSec, floatValue}

		signalSlice = append(signalSlice, singleRowToAdd)
		w.allSignalData[decodedMessage.Topic][signalName] = signalSlice
	}
}

func (w *RawMatlabWriter) GetAllSignalData() map[string]map[string][][]interface{} {
	return w.allSignalData
}
