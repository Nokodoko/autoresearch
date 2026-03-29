package llm

import (
	"fmt"
	"strings"

	"github.com/n0ko/autoresearch/internal/results"
)

// BuildSystemPrompt constructs the system prompt for the LLM.
func BuildSystemPrompt(metricName, direction string) string {
	directionVerb := "lower"
	if direction == "maximize" {
		directionVerb = "higher"
	}

	return fmt.Sprintf(`You are an autonomous research agent optimizing code to improve a scalar metric.

METRIC: %s (%s is better)

YOUR TASK:
1. Analyze the current code and past experiment results
2. Propose ONE targeted, minimal code change
3. The change should test a single hypothesis
4. Avoid changes that have already been tried and failed
5. Prefer simple changes over complex ones
6. Keep changes small and reviewable

OUTPUT FORMAT:
You MUST respond with a JSON object containing:
{
  "target_file": "filename.py",
  "new_content": "... complete file content with your change ...",
  "description": "One-line description of what this change does",
  "reasoning": "Brief explanation of why this should improve the metric"
}

IMPORTANT:
- "new_content" must contain the COMPLETE file content, not just the diff
- Make exactly ONE change per proposal
- Do not add new dependencies
- Do not modify evaluation/metric code
- Keep the code runnable`, metricName, directionVerb)
}

// BuildUserPrompt constructs the user prompt with context.
// ob1History is an optional pre-formatted section from OpenBrain (empty string if disabled).
func BuildUserPrompt(files map[string]string, pastResults []results.Result, bestMetric float64, metricName, ob1History string) string {
	var sb strings.Builder

	// File contents
	sb.WriteString("## Current Code\n\n")
	for name, content := range files {
		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", name, content))
	}

	// Past results (local JSONL log)
	if len(pastResults) > 0 {
		sb.WriteString("## Past Experiment Results\n\n")
		sb.WriteString(fmt.Sprintf("Best %s so far: %.6f\n\n", metricName, bestMetric))

		// Show last 20 results
		start := 0
		if len(pastResults) > 20 {
			start = len(pastResults) - 20
		}

		sb.WriteString("| # | Status | Metric | Description |\n")
		sb.WriteString("|---|--------|--------|-------------|\n")
		for _, r := range pastResults[start:] {
			sb.WriteString(fmt.Sprintf("| %d | %s | %.6f | %s |\n",
				r.Iteration, r.Status, r.MetricValue, r.Description))
		}
		sb.WriteString("\n")
	}

	// OpenBrain experiment history (cross-session memory)
	if ob1History != "" {
		sb.WriteString(ob1History)
	}

	sb.WriteString(fmt.Sprintf("Propose your next change to improve %s. Remember: ONE change, keep it simple.\n", metricName))

	return sb.String()
}
