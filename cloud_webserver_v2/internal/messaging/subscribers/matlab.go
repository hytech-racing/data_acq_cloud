package subscribers

import (
	"fmt"
	"strconv"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"
)

// Really annoying to write but at least im able to pass time on my ✈️

// Plan
// Have one map with all the topics and a nested maps with signals/values
// Have another map with the desired json output
// Have another map with the key as the topic/signal and the value as a string array of path to json output
// After copmletely processing mcap, then only put everything in the json output

type MatlabWriter struct {
	allSignalData map[string]map[string][]float64
	firstLogTime  *float64
	interpValue   float64
	factor        uint16
}

func CreateMatlabWriter(interpolationValue float64) *MatlabWriter {
	i := interpolationValue
	var factor uint16 = 1
	for i < 1 {
		factor *= 10
		i *= 10
	}

	return &MatlabWriter{
		allSignalData: getTopicAndSignalMap(),
		firstLogTime:  nil,
		interpValue:   interpolationValue,
		factor:        factor,
	}
}

func (w *MatlabWriter) constructTopicList() map[string]interface{} {
	return map[string]interface{}{}
}

func (w *MatlabWriter) AddSignalValue(decodedMessage *utils.DecodedMessage) {
	topic := decodedMessage.Topic
	data := decodedMessage.Data
	logTime := float64(decodedMessage.LogTime) / 1e9

	if w.firstLogTime == nil {
		w.firstLogTime = &logTime
	}

	if innerMap, ok := w.allSignalData[topic]; ok {
		for signalName, signalValueInterface := range data {
			if _, ok = innerMap[signalName]; ok {
				fmt.Println(signalName)
				// TODO: Have to put a check here to populate values to interp from the first log time

				signalSlice := w.getSliceWithTopicAndSignal(topic, signalName)
				if signalSlice == nil {
					continue
				}

				signalValueFloat := getFloatValueOfInterface(signalValueInterface)

				if len(signalSlice) == 0 {
					if logTime != *w.firstLogTime {
						w.addInterpolatedValuesToSlice(signalSlice, logTime, signalValueFloat, topic, signalName)
					} else {
						float_val := getFloatValueOfInterface(signalValueFloat)
						signalSlice = append(signalSlice, float_val)
						w.allSignalData[topic][signalName] = signalSlice
					}
				} else {
					w.addInterpolatedValuesToSlice(signalSlice, logTime, signalValueFloat, topic, signalName)
				}
			}
		}
	}
}

func (w *MatlabWriter) addInterpolatedValuesToSlice(signalSlice []float64, logTime float64, value float64, topic string, signalName string) {
	// fmt.Printf("before: %v \n", signalSlice)
	lastSignalTime, lastSignalValue := w.determineLastLogTimeAndSignal(signalSlice)
	if lastSignalValue == nil {
		lastSignalValue = &value
		lastSignalTime = *w.firstLogTime
	}
	interpTime := lastSignalTime

	deltaValue := (value - *lastSignalValue) / (logTime - lastSignalTime)
	// fmt.Printf("value: %f, lastSignalValue: %f, logTime: %f, lastSignalTime: %f, delta %f \n", value, *lastSignalValue, logTime, lastSignalTime, deltaValue)
	for {
		// fmt.Printf("before: %f \n", interpTime)
		// fmt.Printf("should be: %f \n", interpTime+w.interpValue)
		// fmt.Printf("interpvalue is : %f \n", interpTime+w.interpValue)
		interpTime = interpTime + w.interpValue
		// fmt.Printf("after: %f \n \n", interpTime)
		if interpTime > logTime {
			break
		}
		// fmt.Printf("%v, %f, %f, %f \n", value, interpTime, logTime, w.interpValue)

		interpValue := *lastSignalValue + deltaValue*(interpTime-lastSignalTime)
		// fmt.Printf("lastSignalValue: %f, interpTime: %f, lastSignalTime: %f, interpValue \n", *lastSignalValue, interpTime, interpValue, lastSignalTime)
		signalSlice = append(signalSlice, interpValue)

		lastSignalValue = &interpValue
		lastSignalTime = interpTime

		// fmt.Printf("Appended interpolated value: interpTime=%v, interpValue=%v\n", interpTime, interpValue)
		// fmt.Printf("%v, %v \n", interpTime, logTime)
	}

	signalSlice = append(signalSlice, value)
	w.allSignalData[topic][signalName] = signalSlice
	// fmt.Printf("after: %v \n %v \n \n", signalSlice, w.allSignalData)
	// fmt.Printf("Appended final value: logTime=%v, value=%v\n", logTime, value)
}

func (w *MatlabWriter) determineLastLogTimeAndSignal(slice []float64) (float64, *float64) {
	if len(slice) == 0 {
		return float64(*w.firstLogTime), nil
	}

	return float64(*w.firstLogTime) + (float64(len(slice)-1) * w.interpValue), &(slice)[len(slice)-1]
}

func (w *MatlabWriter) getSliceWithTopicAndSignal(topic string, signal string) []float64 {
	if value1, found1 := w.allSignalData[topic]; found1 {
		if value2, found2 := value1[signal]; found2 {
			// fmt.Printf("%v value1: %v, %v \n", value2, value1, w.allSignalData)
			return value2
		}
	}

	return nil
}

func getFloatValueOfInterface(val interface{}) float64 {
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

func (w *MatlabWriter) Get() map[string]map[string][]float64 {
	return w.allSignalData
}

func getTopicAndSignalMap() map[string]map[string][]float64 {
	return map[string]map[string][]float64{
		"global_times": {
			"times": {},
		},
		"acu_shunt_measurements": {
			"current_shunt_read":   {},
			"pack_filtered_read":   {},
			"ts_out_filtered_read": {},
		},
		// "bms_detailed_temps": {
		// 	"current_shunt_read":   {},
		// 	"pack_filtered_read":   {},
		// 	"ts_out_filtered_read": {},
		// },
		// "bms_detailed_voltages": {
		// 	"group_id":  {},
		// 	"ic_id":     {},
		// 	"voltage_0": {},
		// 	"voltage_1": {},
		// 	"voltage_2": {},
		// },
		// "bms_onboard_temps": {
		// 	"average_temp": {},
		// 	"low_temp":     {},
		// 	"high_temp":    {},
		// },
		// "bms_status": {
		// 	"state":                            {},
		// 	"overvoltage_error":                {},
		// 	"undervoltage_error":               {},
		// 	"total_voltage_high_error":         {},
		// 	"discharge_overcurrent_error":      {},
		// 	"charge_overcurrent_error":         {},
		// 	"discharge_overtemp_error":         {},
		// 	"charge_overtemp_error":            {},
		// 	"undertemp_error":                  {},
		// 	"overtemp_error":                   {},
		// 	"current":                          {},
		// 	"shutdown_g_above_threshold_error": {},
		// 	"shutdown_h_above_threshold_error": {},
		// },
		// "bms_temps": {
		// 	"average_temp": {},
		// 	"low_temp":     {},
		// 	"high_temp":    {},
		// },
		// "bms_voltages": {
		// 	"average_voltage": {},
		// 	"low_voltage":     {},
		// 	"high_voltage":    {},
		// 	"total_voltage":   {},
		// },
		// "controller_boolean": {
		// 	"controller_use_launch":            {},
		// 	"controller_use_pid_tv":            {},
		// 	"controller_use_normal_force":      {},
		// 	"controller_use_pid_power_limit":   {},
		// 	"controller_use_power_limit":       {},
		// 	"controller_use_tcs":               {},
		// 	"controller_use_tcs_lim_yaw_pid":   {},
		// 	"controller_use_dec_yaw_pid_brake": {},
		// 	"controller_use_discontin_brakes":  {},
		// 	"controller_use_no_regen_5kph":     {},
		// 	"controller_use_torque_bias":       {},
		// 	"controller_use_nl_tcs_gain_sche":  {},
		// 	"controller_use_rpm_tcs_gain_sche": {},
		// 	"controller_use_nl_tcs_slipschedu": {},
		// },
		// "controller_normal_dist": {
		// 	"controller_normal_percent_fl": {},
		// 	"controller_normal_percent_fr": {},
		// 	"controller_normal_percent_rl": {},
		// 	"controller_normal_percent_rr": {},
		// },
		// "controller_normal_torque": {
		// 	"controller_normal_torque_fl": {},
		// 	"controller_normal_torque_fr": {},
		// 	"controller_normal_torque_rl": {},
		// 	"controller_normal_torque_rr": {},
		// },
	}
}
