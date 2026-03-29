package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/n0ko/autoresearch/internal/config"
	"github.com/n0ko/autoresearch/internal/git"
	"github.com/n0ko/autoresearch/internal/guardrails"
	"github.com/n0ko/autoresearch/internal/llm"
	"github.com/n0ko/autoresearch/internal/loop"
	"github.com/n0ko/autoresearch/internal/metrics"
	"github.com/n0ko/autoresearch/internal/ob1"
	"github.com/n0ko/autoresearch/internal/parallel"
	"github.com/n0ko/autoresearch/internal/results"
	"github.com/n0ko/autoresearch/internal/runner"
	"github.com/n0ko/autoresearch/internal/tui"
	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:     "autoresearch",
		Short:   "Self-improving optimization loop",
		Long:    "autoresearch wraps any experiment script and iteratively optimizes a scalar metric using LLM-proposed code changes.",
		Version: version,
	}

	// --- run command ---
	var runCfgPath string
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Execute the optimization loop",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLoop(cmd, runCfgPath)
		},
	}
	runFlags := runCmd.Flags()
	runFlags.StringP("script", "s", "", "Path to experiment script (required)")
	runFlags.StringP("metric", "m", "", "Metric name to parse from output (required)")
	runFlags.StringP("branch", "b", "", "Git branch for experiments")
	runFlags.IntP("iterations", "i", 0, "Max iterations (0 = unlimited)")
	runFlags.Float64P("threshold", "t", 0.0, "Min improvement to commit")
	runFlags.IntP("parallel", "p", config.DefaultParallel, "Parallel experiment channels")
	runFlags.Duration("timeout", config.DefaultTimeout, "Max time per experiment")
	runFlags.String("llm-backend", config.DefaultLLMBackend, "LLM provider: claude, openai, llamacpp")
	runFlags.String("llm-model", "", "Model name/path")
	runFlags.String("direction", config.DefaultDirection, "Optimization direction: minimize or maximize")
	runFlags.String("target-file", "", "File the LLM should modify")
	runFlags.StringVarP(&runCfgPath, "config", "c", "", "Path to config file")
	runFlags.Bool("no-tui", false, "Disable TUI dashboard")
	runFlags.String("run-command", "", "Command to run the script")
	runFlags.Bool("dry-run", false, "Propose changes but don't execute")

	// --- init command ---
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .autoresearch/ directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}

	// --- status command ---
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current experiment status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}

	// --- results command ---
	resultsCmd := &cobra.Command{
		Use:   "results",
		Short: "Show experiment results",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResults(cmd)
		},
	}
	resultsCmd.Flags().IntP("last", "n", 10, "Show last N results")
	resultsCmd.Flags().Bool("best", false, "Show best result only")
	resultsCmd.Flags().String("export", "", "Export format: json, csv, tsv")

	rootCmd.AddCommand(runCmd, initCmd, statusCmd, resultsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runLoop(cmd *cobra.Command, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override config with flags (flags take precedence)
	flags := cmd.Flags()
	if v, _ := flags.GetString("script"); v != "" {
		cfg.Script = v
	}
	if v, _ := flags.GetString("metric"); v != "" {
		cfg.Metric = v
		if cfg.MetricPattern == "" {
			cfg.MetricPattern = fmt.Sprintf(config.DefaultMetricPatternTemplate, v)
		}
	}
	if v, _ := flags.GetString("branch"); v != "" {
		cfg.Branch = v
	}
	if flags.Changed("iterations") {
		cfg.Iterations, _ = flags.GetInt("iterations")
	}
	if flags.Changed("threshold") {
		cfg.Threshold, _ = flags.GetFloat64("threshold")
	}
	if flags.Changed("parallel") {
		cfg.Parallel, _ = flags.GetInt("parallel")
	}
	if flags.Changed("timeout") {
		cfg.Timeout, _ = flags.GetDuration("timeout")
	}
	if v, _ := flags.GetString("llm-backend"); flags.Changed("llm-backend") {
		cfg.LLMBackend = v
	}
	if v, _ := flags.GetString("llm-model"); v != "" {
		cfg.LLMModel = v
	}
	if v, _ := flags.GetString("direction"); flags.Changed("direction") {
		cfg.Direction = v
	}
	if v, _ := flags.GetString("target-file"); v != "" {
		cfg.TargetFile = v
	}
	if v, _ := flags.GetBool("no-tui"); v {
		cfg.TUI = false
		cfg.NoTUI = true
	}
	if v, _ := flags.GetString("run-command"); v != "" {
		cfg.RunCommand = v
	}
	if v, _ := flags.GetBool("dry-run"); v {
		cfg.DryRun = true
	}

	// Default branch name with timestamp
	if cfg.Branch == "" {
		cfg.Branch = fmt.Sprintf("autoresearch/%s", time.Now().Format("20060102-150405"))
	}

	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal, finishing current experiment...")
		cancel()
	}()

	// Initialize components
	gitOps := git.NewOps(cfg.WorkDir)
	resLog, err := results.NewLog(cfg.ResultsLog)
	if err != nil {
		return fmt.Errorf("initializing results log: %w", err)
	}
	defer resLog.Close()

	metricParser := metrics.NewParser(cfg.MetricPattern, cfg.Direction)
	expRunner := runner.New(cfg.RunCommand, cfg.Script, cfg.Timeout, cfg.WorkDir)
	guard := guardrails.New(cfg)

	provider, err := llm.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("initializing LLM provider: %w", err)
	}

	// Initialize ob1 client (optional, nil if disabled).
	var ob1Client *ob1.Client
	if cfg.OB1URL != "" {
		ob1Client = ob1.NewClient(cfg.OB1URL, cfg.OB1APIKey, cfg.Branch)
		if err := ob1Client.Ping(ctx); err != nil {
			fmt.Printf("Warning: OpenBrain (ob1) unreachable at %s: %v\n", cfg.OB1URL, err)
			fmt.Println("Continuing without ob1 integration.")
			ob1Client = nil
		} else {
			fmt.Printf("Connected to OpenBrain at %s\n", cfg.OB1URL)
		}
	}

	// Build the engine
	engine := loop.NewEngine(cfg, gitOps, resLog, metricParser, expRunner, provider, guard, ob1Client)

	// Run parallel or single-threaded
	if cfg.Parallel > 1 {
		orch := parallel.NewOrchestrator(cfg, engine, gitOps, resLog, ob1Client)
		if cfg.TUI {
			app := tui.NewApp(cfg, orch.Events())
			return app.RunWithOrchestrator(ctx, orch)
		}
		return orch.Run(ctx)
	}

	// Single-threaded mode
	if cfg.TUI {
		app := tui.NewApp(cfg, engine.Events())
		return app.RunWithEngine(ctx, engine)
	}
	return engine.Run(ctx)
}

func runInit() error {
	cfg := &config.Config{}
	cfg.WorkDir, _ = os.Getwd()
	cfg.ConfigDir = config.DefaultConfigDir
	cfg.WorktreeDir = config.DefaultWorktreeDir

	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	// Write default config
	defaultConfig := `# autoresearch configuration
script: ""           # Path to experiment script (required)
metric: ""           # Metric name to parse from output (required)
direction: minimize  # minimize or maximize
branch: ""           # Git branch (auto-generated if empty)
run_command: ""      # Command prefix (e.g., "uv run", "python")
target_file: ""      # File the LLM modifies (auto-detect if empty)

# Execution
iterations: 0        # 0 = unlimited
threshold: 0.0       # Min improvement to commit
parallel: 5          # Parallel experiment channels
timeout: 10m         # Max time per experiment

# LLM
llm_backend: claude  # claude, openai, llamacpp
llm_model: ""        # Empty = provider default
llm_temperature: 0.7

# Guard rails
max_consecutive_failures: 10
max_total_crashes: 20

# TUI
tui: true

# OpenBrain integration (optional)
# ob1_url: "http://localhost:8200"  # uncomment to enable
# ob1_api_key: ""                    # or set AUTORESEARCH_OB1_API_KEY env var
`
	cfgPath := config.DefaultConfigFile
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config already exists at %s\n", cfgPath)
		return nil
	}
	if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0o644); err != nil {
		return fmt.Errorf("writing default config: %w", err)
	}

	fmt.Printf("Initialized .autoresearch/ directory\n")
	fmt.Printf("  Config: %s\n", cfgPath)
	fmt.Printf("  Results: %s (created on first run)\n", config.DefaultResultsFile)
	fmt.Printf("  Worktrees: %s/\n", config.DefaultWorktreeDir)
	return nil
}

func runStatus() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	resLog, err := results.NewLog(cfg.ResultsLog)
	if err != nil {
		return fmt.Errorf("opening results log: %w", err)
	}
	defer resLog.Close()

	all, err := resLog.ReadAll()
	if err != nil {
		return fmt.Errorf("reading results: %w", err)
	}

	if len(all) == 0 {
		fmt.Println("No experiments have been run yet.")
		return nil
	}

	stats := results.ComputeStats(all)
	fmt.Printf("Experiments: %d total (%d keep, %d discard, %d crash)\n",
		stats.Total, stats.Kept, stats.Discarded, stats.Crashed)
	fmt.Printf("Keep rate: %.1f%%\n", stats.KeepRate*100)
	fmt.Printf("Best %s: %.6f\n", all[0].MetricName, stats.BestMetric)
	if stats.Total > 1 {
		fmt.Printf("Baseline: %.6f\n", all[0].MetricValue)
		fmt.Printf("Improvement: %.6f (%.2f%%)\n", stats.Improvement, stats.ImprovementPct*100)
	}
	return nil
}

func runResults(cmd *cobra.Command) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	resLog, err := results.NewLog(cfg.ResultsLog)
	if err != nil {
		return fmt.Errorf("opening results log: %w", err)
	}
	defer resLog.Close()

	showBest, _ := cmd.Flags().GetBool("best")
	last, _ := cmd.Flags().GetInt("last")

	if showBest {
		best, err := resLog.BestResult()
		if err != nil {
			return err
		}
		fmt.Printf("#%d  %s  %.6f  %s\n", best.Iteration, best.Status, best.MetricValue, best.Description)
		return nil
	}

	all, err := resLog.ReadAll()
	if err != nil {
		return err
	}

	start := 0
	if last > 0 && last < len(all) {
		start = len(all) - last
	}

	for _, r := range all[start:] {
		fmt.Printf("#%-4d %-8s %.6f  %s\n", r.Iteration, r.Status, r.MetricValue, r.Description)
	}
	return nil
}
