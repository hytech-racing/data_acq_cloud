package subscribers

import (
	"log"
	"math"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

const EarthRadius = 6371000 // Earth's radius in meters

func LatLonToCartesian(lat, lon, originLat, originLon float64) (float64, float64) {
	// Convert degrees to radians
	lat *= math.Pi / 180
	lon *= math.Pi / 180
	originLat *= math.Pi / 180
	originLon *= math.Pi / 180

	// Calculate differences
	dLat := lat - originLat
	dLon := lon - originLon

	// Calculate x and y
	x := EarthRadius * dLon * math.Cos(originLat)
	y := EarthRadius * dLat

	return x, y
}

func GeneratePlot(xs, ys *[]float64) {
	p := plot.New()
	p.Title.Text = "VN Position Data"
	p.X.Label.Text = "x"
	p.Y.Label.Text = "y"

	err := plotutil.AddScatters(p, "VN Position Data", hplot.ZipXY(*xs, *ys))
	if err != nil {
		log.Fatalf("could not create scatters: %+v", err)
	}

	err = p.Save(25*vg.Centimeter, 25*vg.Centimeter, "./scatter.png")
	if err != nil {
		log.Fatalf("Could not save scatter plot: %+v", err)
	}
}
