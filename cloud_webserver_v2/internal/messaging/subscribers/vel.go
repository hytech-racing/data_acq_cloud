package subscribers

import (
	"fmt"
	"io"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

const PI = 3.14159265359

const GEARBOX_RATIO = 11.86
const WHEEL_DIAMETER = 0.4064 // meters
const RPM_TO_METERS_PER_SECOND = WHEEL_DIAMETER * PI / GEARBOX_RATIO / 60.0
const RPM_TO_KILOMETERS_PER_HOUR = RPM_TO_METERS_PER_SECOND * 3600.0 / 1000.0

// Returns velocity in m/s
func RPMToLinearVelocity(rpm float32) float64 {
	return float64(rpm) * RPM_TO_METERS_PER_SECOND
}

func LogTimeToTime(logTime uint64, initialTime uint64) float64 {
	return float64(logTime-initialTime) / 1e9
}

func GenerateVelPlot(times, vels *[]float64, minTime, maxTime, minVel, maxVel float64) (*io.WriterTo, error) {
	p := plot.New()
	p.Title.Text = "VN Velocity Data"
	p.X.Label.Text = "time (s)"
	p.Y.Label.Text = "velocity (m/s)"
	p.HideAxes()

	// Need to set the max/min for each axis of the plot or else the plot will be stretched.
	// min_value := math.Min(minTime, minVel)
	// max_value := math.Max(maxTime, maxVel)
	p.X.Min = minTime
	p.Y.Min = minVel
	p.X.Max = maxTime
	p.Y.Max = maxVel

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
