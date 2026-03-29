package tui

import (
	"fmt"
	"strings"

	"github.com/n0ko/autoresearch/internal/results"
)

// RenderResultsTable formats results as a simple table string.
func RenderResultsTable(all []results.Result, maxRows int) string {
	if len(all) == 0 {
		return "  No results yet."
	}

	start := 0
	if maxRows > 0 && len(all) > maxRows {
		start = len(all) - maxRows
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %-5s %-8s %-10s %s\n", "#", "Status", "Metric", "Description"))
	sb.WriteString(fmt.Sprintf("  %-5s %-8s %-10s %s\n", "---", "------", "------", "-----------"))

	for _, r := range all[start:] {
		desc := r.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-5d %-8s %-10.6f %s\n",
			r.Iteration, r.Status, r.MetricValue, desc))
	}

	return sb.String()
}
