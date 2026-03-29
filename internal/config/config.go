package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for an autoresearch run.
type Config struct {
	// Core
	Script     string `mapstructure:"script"`
	Metric     string `mapstructure:"metric"`
	Direction  string `mapstructure:"direction"`
	Branch     string `mapstructure:"branch"`
	RunCommand string `mapstructure:"run_command"`
	TargetFile string `mapstructure:"target_file"`

	// Execution
	Iterations int           `mapstructure:"iterations"`
	Threshold  float64       `mapstructure:"threshold"`
	Parallel   int           `mapstructure:"parallel"`
	Timeout    time.Duration `mapstructure:"timeout"`

	// LLM
	LLMBackend     string  `mapstructure:"llm_backend"`
	LLMModel       string  `mapstructure:"llm_model"`
	LLMTemperature float64 `mapstructure:"llm_temperature"`

	// Guard rails
	MaxConsecutiveFailures int `mapstructure:"max_consecutive_failures"`
	MaxTotalCrashes        int `mapstructure:"max_total_crashes"`

	// Metric parsing
	MetricPattern string `mapstructure:"metric_pattern"`

	// TUI
	TUI   bool `mapstructure:"tui"`
	NoTUI bool `mapstructure:"no_tui"`

	// Paths (derived)
	WorkDir    string `mapstructure:"-"`
	ConfigDir  string `mapstructure:"-"`
	ResultsLog string `mapstructure:"-"`
	WorktreeDir string `mapstructure:"-"`

	// Dry run
	DryRun bool `mapstructure:"dry_run"`

	// OpenBrain integration (optional, disabled when OB1URL is empty)
	OB1URL    string `mapstructure:"ob1_url"`
	OB1APIKey string `mapstructure:"ob1_api_key"`
}

// Load reads configuration from file, environment, and applies defaults.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("direction", DefaultDirection)
	v.SetDefault("parallel", DefaultParallel)
	v.SetDefault("timeout", DefaultTimeout)
	v.SetDefault("llm_backend", DefaultLLMBackend)
	v.SetDefault("llm_temperature", DefaultLLMTemperature)
	v.SetDefault("max_consecutive_failures", DefaultMaxConsecFailures)
	v.SetDefault("max_total_crashes", DefaultMaxTotalCrashes)
	v.SetDefault("tui", true)
	v.SetDefault("iterations", 0)
	v.SetDefault("threshold", 0.0)

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(DefaultConfigDir)
		v.AddConfigPath(".")
	}

	// OpenBrain defaults
	v.SetDefault("ob1_url", "")
	v.SetDefault("ob1_api_key", "")

	// Environment variables
	v.SetEnvPrefix("AUTORESEARCH")
	v.AutomaticEnv()

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only error if the file exists but can't be read
			if configPath != "" {
				return nil, fmt.Errorf("reading config %s: %w", configPath, err)
			}
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Derive paths
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	cfg.WorkDir = wd
	cfg.ConfigDir = filepath.Join(wd, DefaultConfigDir)
	cfg.ResultsLog = filepath.Join(wd, DefaultResultsFile)
	cfg.WorktreeDir = filepath.Join(wd, DefaultWorktreeDir)

	// Default metric pattern
	if cfg.MetricPattern == "" && cfg.Metric != "" {
		cfg.MetricPattern = fmt.Sprintf(DefaultMetricPatternTemplate, cfg.Metric)
	}

	// NoTUI overrides TUI
	if cfg.NoTUI {
		cfg.TUI = false
	}

	return cfg, nil
}

// Validate checks that required fields are set and values are valid.
func (c *Config) Validate() error {
	if c.Script == "" {
		return fmt.Errorf("--script is required")
	}
	if c.Metric == "" {
		return fmt.Errorf("--metric is required")
	}
	if c.Direction != "minimize" && c.Direction != "maximize" {
		return fmt.Errorf("--direction must be 'minimize' or 'maximize', got %q", c.Direction)
	}
	if c.Parallel < 1 {
		return fmt.Errorf("--parallel must be >= 1, got %d", c.Parallel)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("--timeout must be > 0")
	}
	if c.LLMBackend != "claude" && c.LLMBackend != "openai" && c.LLMBackend != "llamacpp" {
		return fmt.Errorf("--llm-backend must be 'claude', 'openai', or 'llamacpp', got %q", c.LLMBackend)
	}
	return nil
}

// EnsureDirs creates the .autoresearch directory structure.
func (c *Config) EnsureDirs() error {
	dirs := []string{c.ConfigDir, c.WorktreeDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}
