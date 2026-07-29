package subscribers

import (
	"fmt"
	"io"
	"math"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

const earthRadius = 6371000 // Earth's radius in meters

// LatLonToCartesian converts latitude and longtidue coordinates to a flat 2D LatLonToCartesian plane
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
	x := earthRadius * dLon * math.Cos(originLat)
	y := earthRadius * dLat

	return x, y
}

// GenerateGonumPlot takes in x and y coordinates as well as the bounds of the plot
// in the form of mins and maxs of the X and Y components.
// If successful, it returns a writer to the Gonum plot
func GenerateGonumPlot(xs, ys *[]float64, minX, maxX, minY, maxY float64) (*io.WriterTo, error) {
	p := plot.New()
	p.X.Label.Text = "x"
	p.Y.Label.Text = "y"

	// No valid GPS fix was recorded anywhere in this file (lat/lon stayed at 0
	// the whole run, or every point got filtered out for some other reason).
	// minX/maxX/minY/maxY never moved off their sentinel init values in that
	// case, so plotting them directly would render axes spanning nearly the
	// entire float64 range. Render an explicit "no data" plot instead.
	if len(*xs) == 0 {
		p.Title.Text = "VN Position Data - No GPS Fix Recorded"
		p.X.Min, p.X.Max = -1, 1
		p.Y.Min, p.Y.Max = -1, 1

		label, err := plotter.NewLabels(plotter.XYLabels{
			XYs:    []plotter.XY{{X: 0, Y: 0}},
			Labels: []string{"No GPS fix recorded for this file"},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create no-data label: %+v", err)
		}
		p.Add(label)

		writer, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
		if err != nil {
			return nil, fmt.Errorf("could not get plot writer: %+v", err)
		}
		return &writer, nil
	}

	p.Title.Text = "VN Position Data"

	// Need to set the max/min for each axis of the plot or else the plot will be stretched.
	min_value := math.Min(minX, minY)
	max_value := math.Max(maxX, maxY)
	p.X.Min = min_value
	p.Y.Min = min_value
	p.X.Max = max_value
	p.Y.Max = max_value

	err := plotutil.AddScatters(p, "VN Position Data", hplot.ZipXY(*xs, *ys))
	if err != nil {
		return nil, fmt.Errorf("could not create scatters: %+v", err)
	}

	writer, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
	if err != nil {
		return nil, fmt.Errorf("could not get plot writer: %+v", err)
	}

	return &writer, nil
}
