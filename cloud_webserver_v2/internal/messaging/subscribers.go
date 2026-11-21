package messaging

import (
	"fmt"
	"log"
	"math"
	"os"
	"reflect"

	"github.com/jhump/protoreflect/dynamic"

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
	MAT      = "matlab_writer"
)

// Subscriber function type serves as a common header for all subscribers to a publisher
type SubscriberFunc func(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult)

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
			if decodedFL == nil || decodedFR == nil {
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

func CreateRawMatFile(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	var matWriter *subscribers.RawMatWriter
	var fileName string
	var filePath string
	var conversionErr error
	for msg := range ch {
		if msg.GetContent().Topic == EOF {
			break
		} else if msg.GetContent().Topic == INIT {
			if name, exists := msg.GetContent().Data["file_name"]; exists {
				fileName = name.(string)
			} else {
				log.Printf("no file name was provided for the mat file")
				return
			}

			if path, exists := msg.GetContent().Data["file_path"]; exists {
				filePath = path.(string)
			} else {
				log.Printf("No MATLAB file was generated for %v: no file path was provided", fileName)
				return
			}
			var err error
			matWriter, err = subscribers.CreateRawMatWriter(filePath, fileName)
			if err != nil {
				log.Printf("No MATLAB file was generated for %v: could not start matlab worker: %v", fileName, err)
				return
			}
		} else if matWriter != nil && conversionErr == nil {
			if err := matWriter.AddSignalValue(msg.GetContent()); err != nil {
				conversionErr = err
				log.Printf("could not add signal value to mat file: %v", err)
			}
		}
	}

	if matWriter == nil {
		return
	}

	if conversionErr == nil && matWriter.MaxSignalLength() > 0 {
		if err := matWriter.MatWriter.WriteChunk(matWriter.AllSignalData()); err != nil {
			conversionErr = err
			log.Printf("could not write mat file chunk: %v", err)
		}
	}

	if err := matWriter.MatWriter.Close(); err != nil {
		if conversionErr == nil {
			conversionErr = err
		}
		log.Printf("could not close mat file: %v", err)
	}

	if conversionErr != nil {
		if err := os.Remove(matWriter.FilePath()); err != nil && !os.IsNotExist(err) {
			log.Printf("could not remove incomplete mat file %v: %v", matWriter.FilePath(), err)
		}
		return
	}

	result := make(map[string]interface{})
	result["file_path"] = matWriter.FilePath()
	if results != nil {
		results <- SubscriberResult{SubscriberID: id, SubscriberName: subscriberName, ResultData: result}
	}
}
