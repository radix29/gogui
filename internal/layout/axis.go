package layout

import "github.com/radix29/gogui/internal/driver"

// get returns the component of s on axis a.
func get(s driver.Size, a Axis) float64 {
	if a == Horizontal {
		return s.W
	}
	return s.H
}

// mk builds a size from its main-axis and cross-axis components.
func mk(main Axis, m, c float64) driver.Size {
	if main == Horizontal {
		return driver.Size{W: m, H: c}
	}
	return driver.Size{W: c, H: m}
}

func getPt(p driver.Point, a Axis) float64 {
	if a == Horizontal {
		return p.X
	}
	return p.Y
}

func mkPt(main Axis, m, c float64) driver.Point {
	if main == Horizontal {
		return driver.Point{X: m, Y: c}
	}
	return driver.Point{X: c, Y: m}
}
