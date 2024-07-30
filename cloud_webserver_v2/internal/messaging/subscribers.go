package messaging

import (
	"fmt"
	"math"
	"reflect"

	"github.com/hytech-racing/cloud-webserver-v2/internal/messaging/subscribers"
)

const EOF = "EOF_MESSAGE"

// Subscriber function type
type SubscriberFunc func(id int, ch <-chan SubscribedMessage)

func PrintMessages(id int, ch <-chan SubscribedMessage) {
	for msg := range ch {
		if msg.GetContent().Topic != EOF {
			fmt.Printf("content: %v \n", msg.GetContent().Data)
		}
	}
}

func PlotLatLon(id int, ch <-chan SubscribedMessage) {
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

	subscribers.GeneratePlot(&xs, &ys, minX, maxX, minY, maxY)
}
