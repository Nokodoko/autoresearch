package tui

// Status panel helpers for the TUI dashboard.

import "fmt"

// FormatKeepRate formats the keep rate as a percentage string.
func FormatKeepRate(kept, total int) string {
	if total == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.0f%%", float64(kept)/float64(total)*100)
}

// FormatImprovement formats the metric improvement.
func FormatImprovement(baseline, best float64, direction string) string {
	if direction == "maximize" {
		delta := best - baseline
		if baseline == 0 {
			return "N/A"
		}
		return fmt.Sprintf("%.6f (%.2f%%)", delta, delta/baseline*100)
	}
	delta := baseline - best
	if baseline == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.6f (%.2f%%)", delta, delta/baseline*100)
}
