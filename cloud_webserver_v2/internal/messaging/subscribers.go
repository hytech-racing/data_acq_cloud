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

	slice := matlabWriter.Get()

	for idx := range len(slice) - 1 {
		if slice[idx+1]-slice[idx] != 0.01 {
			fmt.Errorf("Failed, diff is %f", slice[idx+1]-slice[idx])
		}
	}
}
