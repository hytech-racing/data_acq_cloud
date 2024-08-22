package messaging

import (
	"fmt"
	"math"
	"reflect"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging/subscribers"
)

const EOF = "EOF_MESSAGE"

// Subscriber function type
type SubscriberFunc func(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult)

func PrintMessages(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	// mapp := make(map[reflect.Type]bool)
	//
	// for msg := range ch {
	// 	if msg.GetContent().Topic != EOF {
	// 		for _, val := range msg.GetContent().Data {
	// 			typee := reflect.TypeOf(val)
	// 			if num, ok := val.(string); ok {
	// 				i, err := strconv.Atoi(num)
	// 				if err != nil {
	// 					// ... handle error
	// 					panic(err)
	// 				}
	// 				println(i)
	// 			}
	// 			if _, found := mapp[typee]; !found {
	// 				mapp[typee] = true
	// 			}
	// 		}
	//
	// 		// fmt.Printf("content: %v, %v, %v \n", msg.GetContent().Topic, msg.GetContent().Data, reflect.TypeOf(msg.GetContent().Data))
	// 	}
	// }

	// matlab_writer := subscribers.CreateMatlabWriter(0.001)
	// mapp := make(map[string]reflect.Type)
	//
	// for msg := range ch {
	// 	if msg.GetContent().Topic != EOF {
	// 		for key, val := range msg.GetContent().Data {
	// 			if _, found := mapp[key]; !found {
	// 				mapp[msg.content.Topic+"."+key] = reflect.TypeOf(val)
	// 			}
	// 		}
	// 		matlab_writer.AddSignalValue(msg.GetContent())
	// 	}
	// }
	//
	// for key, val := range mapp {
	// 	fmt.Printf("%v, %v \n", key, val)
	// }
	// fmt.Printf("types are %v", mapp)

	for msg := range ch {
		if msg.content.Topic != EOF {
			fmt.Printf("%v \n", msg.content.LogTime)
		}
	}

	result := make(map[string]interface{})
	result["out"] = "Done printing"

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

		if raw, found := data["vn_gps_lat"]; found {
			if lat, ok = raw.(float32); !ok {
				fmt.Errorf("lat is not a float, it is a: %v ", reflect.TypeOf(lat))
			}
		}

		if raw, found := data["vn_gps_lon"]; found {
			if lon, ok = raw.(float32); !ok {
				fmt.Errorf("lon is not a float, it is a: %v ", reflect.TypeOf(lon))
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

func CreateMatlabFile(id int, subscriberName string, ch <-chan SubscribedMessage, results chan<- SubscriberResult) {
	matlabWriter := subscribers.CreateMatlabWriter(0.01)

	fmt.Println("init writer")
	for msg := range ch {
		if msg.GetContent().Topic == EOF {
			break
		}

		matlabWriter.AddSignalValue(msg.GetContent())
	}

	fmt.Printf("Interpolated list is %v \n", matlabWriter.Get())

	// *thing1 := make(map[string]*[]interface{})
	//
	//	thing2 := make(map[string]*[]interface{})
	//
	//	list := []int{1, 2, 3}
	//
	//	// Convert []int to []interface{}
	//	interfaceList := make([]interface{}, len(list))
	//	for i, v := range list {
	//		interfaceList[i] = v
	//	}
	//
	//	thing1["a"] = &interfaceList
	//	thing2["b"] = &interfaceList
	//
	//	if x, found := thing2["b"]; found {
	//		*x = append(*x, 4)
	//	}
	//
	//	if t1, found := thing1["a"]; found {
	//		fmt.Printf("%v \n", t1)
	//	}
	//
	//	if t2, found := thing2["b"]; found {
	//		fmt.Printf("%v \n", *t2)
	//	}
}
