package metrics

import "math"

// WorstValue returns the worst possible metric value for the given direction.
func WorstValue(direction string) float64 {
	if direction == "maximize" {
		return math.Inf(-1)
	}
	return math.Inf(1)
}

// BestOf returns the better of two values for the given direction.
func BestOf(a, b float64, direction string) float64 {
	if direction == "maximize" {
		if a > b {
			return a
		}
		return b
	}
	if a < b {
		return a
	}
	return b
}
