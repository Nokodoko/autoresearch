package config

import "time"

const (
	DefaultParallel             = 5
	DefaultTimeout              = 10 * time.Minute
	DefaultDirection            = "minimize"
	DefaultLLMBackend           = "claude"
	DefaultLLMTemperature       = 0.7
	DefaultMaxConsecFailures    = 10
	DefaultMaxTotalCrashes      = 20
	DefaultMetricPatternTemplate = `^%s:\s+([\d.]+)`
	DefaultConfigDir            = ".autoresearch"
	DefaultConfigFile           = ".autoresearch/config.yaml"
	DefaultResultsFile          = ".autoresearch/results.jsonl"
	DefaultWorktreeDir          = ".autoresearch/worktrees"
)
