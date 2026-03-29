package ob1

import (
	"fmt"
	"strings"
)

// FormatHistory formats ob1 experiment entries into a markdown section
// suitable for inclusion in the LLM user prompt. Returns an empty string
// if there are no entries.
func FormatHistory(entries []ExperimentEntry, metricName string) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## OpenBrain Experiment History\n\n")
	sb.WriteString("These are past experiment results stored in OpenBrain (organizational memory).\n")
	sb.WriteString("Use this history to avoid repeating failed approaches and build on successful ones.\n\n")

	sb.WriteString("| # | Status | Metric | Description |\n")
	sb.WriteString("|---|--------|--------|-------------|\n")

	// Show most recent entries (entries are already ordered by captured_at DESC).
	maxEntries := 30
	if len(entries) < maxEntries {
		maxEntries = len(entries)
	}

	for _, e := range entries[:maxEntries] {
		desc := e.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %.6f | %s |\n",
			e.Iteration, e.Status, e.MetricValue, desc))
	}
	sb.WriteString("\n")

	if len(entries) > maxEntries {
		sb.WriteString(fmt.Sprintf("(%d more entries in OpenBrain, showing latest %d)\n\n", len(entries)-maxEntries, maxEntries))
	}

	return sb.String()
}
