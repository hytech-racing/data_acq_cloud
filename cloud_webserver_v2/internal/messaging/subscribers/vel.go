package subscribers

import (
	"fmt"
	"io"
	"math"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func RPMToLinearVelocity(rpm, wheelDiameter float32) float64 {
	// RPM math here
	return 0.0
}

func LogTimeToTime(logTime uint64) float64 {
	// RPM math here
	return 0.0
}

func GenerateVelPlot(times, vels *[]float64, minTime, maxTime, minVel, maxVel float64) (*io.WriterTo, error) {
	p := plot.New()
	p.Title.Text = "VN Velocity Data"
	p.X.Label.Text = "time"
	p.Y.Label.Text = "velocity"
	p.HideAxes()

	// Need to set the max/min for each axis of the plot or else the plot will be stretched.
	min_value := math.Min(minTime, minVel)
	max_value := math.Max(maxTime, maxVel)
	p.X.Min = min_value
	p.Y.Min = min_value
	p.X.Max = max_value
	p.Y.Max = max_value

	err := plotutil.AddScatters(p, "VN Velocity Data", hplot.ZipXY(*times, *vels))
	if err != nil {
		return nil, fmt.Errorf("could not create scatters: %+v", err)
	}

	writer, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
	if err != nil {
		return nil, fmt.Errorf("could not get plot writer: %+v", err)
	}

	return &writer, nil
}
