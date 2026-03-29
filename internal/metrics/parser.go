package metrics

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parser extracts a scalar metric from experiment output.
type Parser struct {
	pattern   *regexp.Regexp
	direction string // "minimize" or "maximize"
}

// NewParser creates a metric parser with the given regex pattern and direction.
// The pattern must have exactly one capture group for the numeric value.
func NewParser(pattern, direction string) *Parser {
	return &Parser{
		pattern:   regexp.MustCompile(pattern),
		direction: direction,
	}
}

// Parse scans output text for the metric value.
// Returns the metric value and nil error if found.
func (p *Parser) Parse(output string) (float64, error) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		matches := p.pattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) >= 2 {
			val, err := strconv.ParseFloat(strings.TrimSpace(matches[1]), 64)
			if err != nil {
				continue
			}
			return val, nil
		}
	}
	return 0, fmt.Errorf("metric not found in output (pattern: %s)", p.pattern.String())
}

// IsBetter returns true if newVal is better than oldVal according to the direction.
func (p *Parser) IsBetter(newVal, oldVal float64) bool {
	if p.direction == "maximize" {
		return newVal > oldVal
	}
	return newVal < oldVal // minimize
}

// IsBetterByThreshold returns true if newVal is better than oldVal by at least threshold.
func (p *Parser) IsBetterByThreshold(newVal, oldVal, threshold float64) bool {
	if p.direction == "maximize" {
		return (newVal - oldVal) >= threshold
	}
	return (oldVal - newVal) >= threshold // minimize
}

// Direction returns the optimization direction.
func (p *Parser) Direction() string {
	return p.direction
}
