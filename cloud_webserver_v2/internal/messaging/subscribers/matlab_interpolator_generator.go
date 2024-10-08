package subscribers

import (
	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

// Really annoying to write but at least im able to pass time on my ✈️

// new idea -> have all the data in the slices. Then just do another scan opf the inteprolated times and tkae the average of the 2 bounds

type InterpolatedMatlabWriter struct {
	allSignalData map[string]map[string][]float64
	firstLogTime  *float64
	interpValue   float64
	factor        uint16
}

func CreateInterpolatedMatlabWriter(interpolationValue float64, schema map[string]map[string][]float64) *InterpolatedMatlabWriter {
	i := interpolationValue
	var factor uint16 = 1
	for i < 1 {
		factor *= 10
		i *= 10
	}

	return &InterpolatedMatlabWriter{
		allSignalData: schema,
		firstLogTime:  nil,
		interpValue:   interpolationValue,
		factor:        factor,
	}
}

func (w *InterpolatedMatlabWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) {
	topic := decodedMessage.Topic
	data := decodedMessage.Data
	logTime := float64(decodedMessage.LogTime) / 1e9

	if w.firstLogTime == nil {
		w.firstLogTime = &logTime
		timeSlice := w.getLogTimeSlice()
		timeSlice = append(timeSlice, *w.firstLogTime)
		w.allSignalData["global_times"]["times"] = timeSlice
	}

	if innerMap, ok := w.allSignalData[topic]; ok {
		for signalName, signalValueInterface := range data {
			if _, ok = innerMap[signalName]; !ok {
				innerMap[signalName] = make([]float64, 0)
			}

			signalSlice := w.getSliceWithTopicAndSignal(topic, signalName)
			if signalSlice == nil {
				continue
			}

			signalValueFloat := utils.GetFloatValueOfInterface(signalValueInterface)

			if len(signalSlice) == 0 {
				if logTime != *w.firstLogTime {
					w.addInterpolatedValuesToSlice(signalSlice, logTime, signalValueFloat, topic, signalName)
				} else {
					signalSlice = append(signalSlice, signalValueFloat)
					w.allSignalData[topic][signalName] = signalSlice
				}
			} else {
				w.addInterpolatedValuesToSlice(signalSlice, logTime, signalValueFloat, topic, signalName)
			}
		}
	}
}

func (w *InterpolatedMatlabWriter) addInterpolatedValuesToSlice(signalSlice []float64, logTime float64, value float64, topic string, signalName string) {
	lastSignalTime, lastSignalValue := w.determineLastLogTimeAndSignal(signalSlice)
	timeSlice := w.getLogTimeSlice()
	interpTime := lastSignalTime + w.interpValue

	if lastSignalValue == nil {
		lastSignalValue = &value
		lastSignalTime = *w.firstLogTime
		signalSlice = append(signalSlice, *lastSignalValue)
		interpTime += w.interpValue
	}

	deltaValue := (value - *lastSignalValue) / (logTime - lastSignalTime)

	for interpTime <= logTime {
		interpValue := *lastSignalValue + deltaValue*(interpTime-lastSignalTime)
		signalSlice = append(signalSlice, interpValue)

		lastSignalValue = &interpValue
		lastSignalTime = interpTime

		for len(timeSlice) != 0 && timeSlice[len(timeSlice)-1] < interpTime {
			timeSlice = append(timeSlice, interpTime)
		}

		interpTime += w.interpValue
	}

	w.allSignalData[topic][signalName] = signalSlice
	w.allSignalData["global_times"]["times"] = timeSlice
}

func (w *InterpolatedMatlabWriter) getLogTimeSlice() []float64 {
	return w.getSliceWithTopicAndSignal("global_times", "times")
}

func (w *InterpolatedMatlabWriter) determineLastLogTimeAndSignal(slice []float64) (float64, *float64) {
	if len(slice) == 0 {
		return float64(*w.firstLogTime), nil
	}

	return float64(*w.firstLogTime) + (float64(len(slice)-1) * w.interpValue), &(slice)[len(slice)-1]
}

func (w *InterpolatedMatlabWriter) getSliceWithTopicAndSignal(topic string, signal string) []float64 {
	if value1, found1 := w.allSignalData[topic]; found1 {
		if value2, found2 := value1[signal]; found2 {
			return value2
		}
	}

	return nil
}

func (w *InterpolatedMatlabWriter) InterpolateEndOfSignalSlices() {
	lenTimeSlice := len(w.allSignalData["global_times"]["times"])
	for topic, signalNameMap := range w.allSignalData {
		for signalName, signalSlice := range signalNameMap {
			if signalSlice == nil {
				continue
			}

			for len(signalSlice) < lenTimeSlice {
				signalSlice = append(signalSlice, signalSlice[len(signalSlice)-1])
			}

			w.allSignalData[topic][signalName] = signalSlice
		}
	}
}

func (w *InterpolatedMatlabWriter) GetAllSignalData() map[string]map[string][]float64 {
	return w.allSignalData
}

func (w *InterpolatedMatlabWriter) GetSignalLengths(topic string, signal string) int {
	return len(w.allSignalData[topic][signal])
}
