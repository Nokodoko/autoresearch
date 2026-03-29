package tui

import (
	"fmt"
	"math"
	"strings"
)

// renderChart draws a simple ASCII chart of metric values.
func renderChart(values []float64, width, height int) string {
	if len(values) == 0 || width < 10 || height < 3 {
		return ""
	}

	// Find min/max
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Add small margin
	spread := maxVal - minVal
	if spread == 0 {
		spread = 1
	}
	margin := spread * 0.1
	minVal -= margin
	maxVal += margin

	// Label width
	labelWidth := 10

	// Chart area dimensions
	chartWidth := width - labelWidth - 2
	if chartWidth < 5 {
		chartWidth = 5
	}

	// Build grid
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, chartWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Plot points
	for i, v := range values {
		x := i * (chartWidth - 1) / max(len(values)-1, 1)
		y := int(float64(height-1) * (maxVal - v) / (maxVal - minVal))
		if y < 0 {
			y = 0
		}
		if y >= height {
			y = height - 1
		}
		if x >= 0 && x < chartWidth {
			grid[y][x] = '*'
		}
	}

	// Render
	var sb strings.Builder
	for row := 0; row < height; row++ {
		// Y-axis label
		yVal := maxVal - float64(row)*(maxVal-minVal)/float64(height-1)
		label := fmt.Sprintf("%*.4f", labelWidth, yVal)
		sb.WriteString(label)
		sb.WriteString(" |")
		sb.WriteString(string(grid[row]))
		sb.WriteString("\n")
	}

	// X-axis
	sb.WriteString(strings.Repeat(" ", labelWidth))
	sb.WriteString(" +")
	sb.WriteString(strings.Repeat("-", chartWidth))
	sb.WriteString("\n")

	// X-axis labels
	sb.WriteString(strings.Repeat(" ", labelWidth+2))
	sb.WriteString(fmt.Sprintf("0"))
	if len(values) > 1 {
		gap := chartWidth - len(fmt.Sprintf("%d", len(values)-1)) - 1
		if gap > 0 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
		sb.WriteString(fmt.Sprintf("%d", len(values)-1))
	}

	return sb.String()
}
