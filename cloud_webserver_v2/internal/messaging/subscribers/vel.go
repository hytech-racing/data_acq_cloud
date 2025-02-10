package subscribers

import (
	"fmt"
	"io"
	"math"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

const GearboxRatio = 11.86
const WheelDiameter = 0.4064 // meters
const RpmToMetersPerSecond = WheelDiameter * math.Pi / GearboxRatio / 60.0
const RpmToKilometersPerHour = RpmToMetersPerSecond * 3600.0 / 1000.0

func RPMToLinearVelocity(rpm float32) float64 {
	return float64(rpm) * RpmToMetersPerSecond
}

func LogTimeToTime(logTime uint64, initialTime uint64) float64 {
	return float64(logTime-initialTime) / 1e9
}

func GenerateVelocityPlot(times, vels *[]float64, minTime, maxTime, minVel, maxVel float64) (*io.WriterTo, error) {
	p := plot.New()
	p.Title.Text = "VN Velocity Data"
	p.X.Label.Text = "time (s)"
	p.Y.Label.Text = "velocity (m/s)"
	p.HideAxes()

	p.X.Min = minTime
	p.Y.Min = minVel
	p.X.Max = maxTime
	p.Y.Max = maxVel

	pts := make(plotter.XYs, len(*times))
	for i := range *times {
		pts[i].X = (*times)[i]
		pts[i].Y = (*vels)[i]
	}

	line, err := plotter.NewLine(pts)
	if err != nil {
		return nil, fmt.Errorf("could not create line plot: %+v", err)
	}
	p.Add(line)

	writer, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
	if err != nil {
		return nil, fmt.Errorf("could not get plot writer: %+v", err)
	}

	return &writer, nil
}
