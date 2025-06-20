package messaging

import (
	"fmt"
	"log"
	"math"
	"reflect"

	"github.com/jhump/protoreflect/dynamic"

	"github.com/hytech-racing/cloud-webserver-v2/internal/utils"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging/subscribers"
)

/*
This is where all the subscribers live.
NOTE: NOT ALL OF THESE ARE RAN AT THE SAME TIME.
Any publisher can send messages to any combination of these subscribers.
Lots of the internal logic for the subscribers lives in the messaging/subscribers directory.
*/

const (
	EOF  = "EOF_MESSAGE"
	INIT = "INIT_MESSAGE"
)

const (
	LATLON   = "vn_plot"
	VELOCITY = "velocity_plot"
	MATLAB   = "matlab_writer"
)

// Subscriber function type serves as a common header for all subscribers to a publisher
type SubscriberFunc func(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult)

func PrintMessages(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	mx_accel := 0.0
	mx_y_accel := 0.0
	for msg := range ch {
		println(msg.content.Data)
	}

	result := make(map[string]interface{})
	result["out"] = "Done printing"

	fmt.Println(mx_accel)
	fmt.Println(mx_y_accel)
	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}

func PlotLatLon(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	xs := make([]float64, 0)
	ys := make([]float64, 0)
	first := true
	var originLon, originLat float64
	minX, maxX, minY, maxY := math.MaxFloat64, math.SmallestNonzeroFloat64, math.MaxFloat64, math.SmallestNonzeroFloat64

	for msg := range ch {
		if msg.GetContent().Topic == EOF {
			break
		}

		data := msg.GetContent().Data

		var lat float32
		var lon float32
		var ok bool

		if gpsDynamicMessage, found := data["vn_gps"].(*dynamic.Message); found {
			latFieldDescriptor := gpsDynamicMessage.FindFieldDescriptorByName("lat")
			lonFieldDescriptor := gpsDynamicMessage.FindFieldDescriptorByName("lon")
			if latFieldDescriptor == nil || lonFieldDescriptor == nil {
				continue
			}

			decodedLat := gpsDynamicMessage.GetField(latFieldDescriptor)
			decodedLon := gpsDynamicMessage.GetField(lonFieldDescriptor)
			if decodedLat == nil || decodedLon == nil {
				continue
			}

			if lat, ok = decodedLat.(float32); !ok {
				log.Printf("lat is not a float, it is a: %v \n", reflect.TypeOf(lat))
				continue
			}
			if lon, ok = decodedLon.(float32); !ok {
				log.Printf("lon is not a float, it is a: %v \n", reflect.TypeOf(lon))
				continue
			}

		}

		if lat == 0 || lon == 0 {
			continue
		}

		if first {
			originLat = float64(lat)
			originLon = float64(lon)
			first = false
		}

		x, y := subscribers.LatLonToCartesian(float64(lat), float64(lon), originLat, originLon)

		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)

		xs = append(xs, x)
		ys = append(ys, y)

	}

	writerTo, err := subscribers.GenerateGonumPlot(&xs, &ys, minX, maxX, minY, maxY)
	if err != nil {
		log.Println(err)
		return
	}

	result := make(map[string]interface{})
	result["writer_to"] = writerTo

	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}

func PlotTimeVelocity(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	times := make([]float64, 0)
	vels := make([]float64, 0)
	first := true
	var initialTime uint64
	minTime, maxTime, minVel, maxVel := math.MaxFloat64, math.SmallestNonzeroFloat64, math.MaxFloat64, math.SmallestNonzeroFloat64

	for msg := range ch {
		if msg.GetContent().Topic == EOF {
			break
		}

		data := msg.GetContent().Data

		var fl float32
		var fr float32
		var rpm float32
		var logTime uint64
		var ok bool

		if veh_vec_floatDynamicMessage, found := data["current_rpms"].(*dynamic.Message); found {
			fl_Descriptor := veh_vec_floatDynamicMessage.FindFieldDescriptorByName("FL")
			fr_Descriptor := veh_vec_floatDynamicMessage.FindFieldDescriptorByName("FR")

			if fl_Descriptor == nil || fr_Descriptor == nil {
				continue
			}

			decodedFL := veh_vec_floatDynamicMessage.GetField(fl_Descriptor)
			decodedFR := veh_vec_floatDynamicMessage.GetField(fr_Descriptor)
			if decodedFL == nil {
				continue
			}

			if fl, ok = decodedFL.(float32); !ok {
				log.Printf("fl is not a float, it is a: %v \n", reflect.TypeOf(fl))
				continue
			}
			if fr, ok = decodedFR.(float32); !ok {
				log.Printf("fr is not a float, it is a: %v \n", reflect.TypeOf(fr))
				continue
			}

			rpm = fr
			logTime = msg.GetContent().LogTime
		}

		if rpm == 0 {
			continue
		}

		if first {
			initialTime = logTime
			first = false
		}

		vel := subscribers.RPMToLinearVelocity(rpm)
		time := subscribers.LogTimeToTime(logTime, initialTime)

		minVel = math.Min(minVel, vel)
		maxVel = math.Max(maxVel, vel)
		minTime = math.Min(minTime, time)
		maxTime = math.Max(maxTime, time)

		vels = append(vels, vel)
		times = append(times, time)
	}

	writerTo, err := subscribers.GenerateVelocityPlot(&times, &vels, minTime, maxTime, minVel, maxVel)
	if err != nil {
		log.Println(err)
		return
	}

	result := make(map[string]interface{})
	result["writer_to"] = writerTo

	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}

func CreateInterpolatedMatlabFile(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	var matlabWriter *subscribers.InterpolatedMatlabWriter
	for msg := range ch {
		if msg.GetContent().Topic == EOF {
			break
		} else if msg.GetContent().Topic == INIT {
			schema, err := getInterpolatedSchemaMap(&msg)
			if err != nil {
				log.Panic("could not get mcap schema map")
			}
			matlabWriter = subscribers.CreateInterpolatedMatlabWriter(0.001, schema)
		} else {
			if matlabWriter != nil {
				matlabWriter.AddSignalValue(msg.GetContent())
			}
		}
	}

	if matlabWriter != nil {
		matlabWriter.InterpolateEndOfSignalSlices()
	}

	result := make(map[string]interface{})
	allSignalData := matlabWriter.GetAllSignalData()
	result["interpolated_data"] = &allSignalData

	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}

func CreateRawMatlabFile(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	var matlabWriter *subscribers.RawMatlabWriter
	var fileName string
	var filePath string
	for msg := range ch {
		if msg.GetContent().Topic == EOF {
		} else if msg.GetContent().Topic == INIT {
			if name, exists := msg.GetContent().Data["file_name"]; exists {
				fileName = name.(string)
			} else {
				break
			}

			if path, exists := msg.GetContent().Data["file_path"]; exists {
				filePath = path.(string)
			} else {
				break
			}
			var err error
			matlabWriter, err = subscribers.CreateRawMatlabWriter(filePath, fileName)
			if err != nil {
				log.Printf("could not start matlab worker: %v", err)
				break
			}
		} else {
			if matlabWriter != nil {
				err := matlabWriter.AddSignalValue(msg.GetContent())
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	if matlabWriter.MaxSignalLength() > 0 {
		err := matlabWriter.HDF5Writer.ChunkWrite(matlabWriter.AllSignalData())
		if err != nil {
			log.Printf("could not chunk write hdf5 file: %v", err)
		}

		err = matlabWriter.HDF5Writer.Close()
		if err != nil {
			log.Printf("could not close hdf5 file: %v", err)
		}
	}

	result := make(map[string]interface{})
	result["file_path"] = matlabWriter.FilePath()
	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}

func getInterpolatedSchemaMap(message *SubscribedMessage) (map[string]map[string][]float64, error) {
	data := message.GetContent().Data

	if schemas, found := data["schema_list"]; found {
		if reflect.TypeOf(schemas) != reflect.SliceOf(reflect.TypeOf("")) {
			return nil, fmt.Errorf("correct schema is not provided for matlab generation")
		}

		return utils.GetMcapSchemaMap(schemas.([]string))
	}

	return nil, fmt.Errorf("correct schema is not provided for matlab generation")
}
