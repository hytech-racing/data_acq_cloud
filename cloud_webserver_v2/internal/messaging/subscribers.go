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
KEY NOTE: NOT ALL OF THESE ARE RAN AT THE SAME TIME.
Any publisher can send messages to any combination of these subscribers.
Lots of the internal logic for the subscribers lives in the messaging/subscribers directory.
*/

const (
	EOF  = "EOF_MESSAGE"
	INIT = "INIT_MESSAGE"
)

// Subscriber function type
type SubscriberFunc func(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult)

func PrintMessages(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	mx_accel := 0.0
	mx_y_accel := 0.0
	for msg := range ch {
		if msg.content.Topic != EOF {
			// fmt.Printf("%v \n", msg.content.Topic)
		}
		if msg.content.Topic == EOF {
			fmt.Println("EOF LMAO")
		}
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
				log.Println("lat is not a float, it is a: %v ", reflect.TypeOf(lat))
				continue
			}
			if lon, ok = decodedLon.(float32); !ok {
				log.Println("lon is not a float, it is a: %v ", reflect.TypeOf(lon))
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

	writerTo := subscribers.GeneratePlot(&xs, &ys, minX, maxX, minY, maxY)

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
