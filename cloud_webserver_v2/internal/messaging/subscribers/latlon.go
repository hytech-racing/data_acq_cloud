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

func GeneratePlot(xs, ys *[]float64, minX, maxX, minY, maxY float64) {
	p := plot.New()
	p.Title.Text = "VN Position Data"
	p.X.Label.Text = "x"
	p.Y.Label.Text = "y"
	p.HideAxes()

	min_value := math.Min(minX, minY)
	max_value := math.Max(maxX, maxY)
	p.X.Min = min_value
	p.Y.Min = min_value
	p.X.Max = max_value
	p.Y.Max = max_value

	log.Printf("minimum is %f, %f \n", min_value, max_value)
	log.Printf("x: %f -> %f, y: %f -> %f \n", p.X.Min, p.X.Max, p.Y.Min, p.Y.Max)

	err := plotutil.AddScatters(p, "VN Position Data", hplot.ZipXY(*xs, *ys))
	if err != nil {
		log.Fatalf("could not create scatters: %+v", err)
	}

	err = p.Save(25*vg.Centimeter, 25*vg.Centimeter, "./scatter2-fix.png")
	if err != nil {
		log.Fatalf("Could not save scatter plot: %+v", err)
	}
}
