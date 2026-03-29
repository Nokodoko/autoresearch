# autoresearch Go Orchestrator -- Specification

## Overview

`autoresearch` is a Go CLI tool that implements a self-improving optimization loop. It wraps any experiment script (e.g., `train.py`), iteratively proposes code changes via an LLM, runs experiments, parses a scalar metric, and commits improvements to git. It supports parallel experiment execution via git worktrees and Go channels.

The tool is **generalized** -- it works with any iterative optimization problem that has:
1. A script to run
2. A scalar metric to optimize (lower or higher is better, configurable)
3. A codebase to modify

## CLI Interface

```
autoresearch run [flags]

Flags:
  --script, -s        string   Path to experiment script (required)
  --metric, -m        string   Metric name to parse from output (required, e.g. "val_bpb")
  --branch, -b        string   Git branch for experiments (default: "autoresearch/<timestamp>")
  --iterations, -i    int      Max iterations before halting (default: 0 = unlimited)
  --threshold, -t     float64  Min improvement to commit (default: 0.0 = any improvement)
  --parallel, -p      int      Number of parallel experiment channels (default: 5)
  --timeout           duration Max time per experiment run (default: 10m)
  --llm-backend       string   LLM provider: "claude", "openai", "llamacpp" (default: "claude")
  --llm-model         string   Model name/path (default: provider-specific)
  --direction          string   Optimization direction: "minimize" or "maximize" (default: "minimize")
  --target-file       string   File the LLM should modify (default: auto-detect)
  --config, -c        string   Path to config file (default: ".autoresearch/config.yaml")
  --no-tui            bool     Disable TUI dashboard, use simple line output (default: false)
  --run-command       string   Command to run the script (default: auto-detect, e.g. "uv run")
  --dry-run           bool     Propose changes but don't execute (default: false)

autoresearch init [flags]
  Initialize .autoresearch/ directory with default config

autoresearch status
  Show current experiment status, results summary

autoresearch results [flags]
  --last, -n          int      Show last N results (default: 10)
  --best              bool     Show best result only
  --export            string   Export format: "json", "csv", "tsv"
```

## Project Layout

```
/home/n0ko/Programs/autoresearch/
  go.mod
  go.sum
  cmd/
    autoresearch/
      main.go              # Entry point, cobra root command
  internal/
    config/
      config.go            # Configuration struct, loading (flags + YAML + env)
      defaults.go          # Default values
    loop/
      engine.go            # Core loop: read -> propose -> run -> score -> commit/revert
      context.go           # Context builder: reads git log, results, file state
    llm/
      provider.go          # Provider interface
      claude.go            # Anthropic Claude API
      openai.go            # OpenAI-compatible API
      llamacpp.go          # Local llama.cpp wrapper
      prompt.go            # System/user prompt construction
      patch.go             # Patch struct and parsing
    git/
      ops.go               # Core git operations (commit, revert, branch, log)
      worktree.go          # Worktree create/remove/list/merge
      diff.go              # Patch application
    runner/
      runner.go            # Experiment execution (exec.Command + output capture)
      timeout.go           # Timeout and process management
    metrics/
      parser.go            # Metric extraction from script output
      compare.go           # Metric comparison (minimize vs maximize)
    parallel/
      orchestrator.go      # Fan-out/fan-in parallel experiment manager
      channel.go           # Channel-based experiment coordination
    results/
      log.go               # Append-only JSONL results log
      reader.go            # Results reading and querying
      stats.go             # Summary statistics
    tui/
      app.go               # Bubbletea application
      chart.go             # ASCII metric curve
      table.go             # Results table
      status.go            # Status panel
    guardrails/
      guardrails.go        # Max iterations, threshold, timeout, crash rollback
      shutdown.go          # Graceful shutdown (SIGINT/SIGTERM)
  .autoresearch/
    config.yaml            # Default configuration
    results.jsonl           # Experiment results log (created at runtime)
    worktrees/             # Git worktrees for parallel experiments (created at runtime)
```

## Core Loop

The fundamental loop (single-threaded version):

```
1. READ CONTEXT
   - Read current state of target file(s)
   - Read git log (last N commits on experiment branch)
   - Read results.jsonl (all past experiments: metric, status, description)
   - Build context string for LLM

2. PROPOSE EDIT
   - Send context to LLM with system prompt
   - System prompt includes: optimization goal, metric name, direction, constraints,
     past results summary, current file content
   - LLM returns a Patch: file path, unified diff, description

3. APPLY PATCH
   - Apply the unified diff to the target file in the working directory (or worktree)
   - Validate the patch applied cleanly

4. RUN EXPERIMENT
   - Execute: <run-command> <script> (e.g., "uv run train.py")
   - Capture stdout + stderr to a buffer
   - Enforce timeout (kill process if exceeded)
   - Detect crashes (non-zero exit code)

5. PARSE METRIC
   - Scan output for metric line matching pattern: "<metric_name>:\s+<value>"
   - Extract scalar float64 value
   - If metric not found, treat as crash

6. DECIDE: COMMIT OR REVERT
   - Compare new metric against best known metric
   - If improved (by at least --threshold): git commit with descriptive message, log as "keep"
   - If not improved: git reset --hard, log as "discard"
   - If crashed: git reset --hard, log as "crash"
   - Append result to results.jsonl

7. CHECK GUARD RAILS
   - If --iterations reached: halt
   - If SIGINT received: finish current, halt
   - Otherwise: goto 1
```

## Parallel Execution

When `--parallel > 1`, the orchestrator runs N experiments concurrently:

```
1. DISPATCH PHASE
   - Create N git worktrees from current HEAD: .autoresearch/worktrees/exp-<iteration>-<channel>/
   - For each worktree, request a unique proposal from the LLM
     (each proposal sees the same context but LLM temperature ensures diversity)
   - Apply each patch in its respective worktree
   - Launch N experiment processes (one per worktree)

2. COLLECT PHASE
   - Wait for all N experiments to complete (with timeout)
   - Parse metrics from each
   - Rank results by metric improvement

3. MERGE PHASE
   - For experiments that improved the metric (sorted by improvement, best first):
     a. Attempt merge of worktree branch into main experiment branch
     b. If merge clean: accept, update HEAD, log as "keep"
     c. If merge conflicts: skip (log as "conflict"), try next
   - For experiments that did not improve or crashed: log as "discard"/"crash"

4. CLEANUP PHASE
   - Remove all worktrees for this iteration
   - Update context with new HEAD state
   - Proceed to next iteration
```

## Results Log Format

File: `.autoresearch/results.jsonl`

Each line is a JSON object:

```json
{
  "timestamp": "2026-03-29T14:30:00Z",
  "iteration": 1,
  "channel": 0,
  "commit": "a1b2c3d",
  "metric_name": "val_bpb",
  "metric_value": 0.997900,
  "best_metric": 0.997900,
  "status": "keep",
  "duration_seconds": 305.2,
  "description": "baseline run",
  "patch_summary": "",
  "worktree": "",
  "error": ""
}
```

Fields:
- `timestamp`: RFC3339 timestamp of result logging
- `iteration`: Loop iteration number (0-indexed)
- `channel`: Parallel channel ID (0 for single-threaded)
- `commit`: Short git commit hash (7 chars) or empty for crashes
- `metric_name`: Name of the metric being optimized
- `metric_value`: Scalar metric value (0.0 for crashes)
- `best_metric`: Best metric value seen so far
- `status`: "keep", "discard", "crash", or "conflict"
- `duration_seconds`: Wall-clock time of experiment run
- `description`: LLM-generated description of the change
- `patch_summary`: Brief summary of the code diff
- `worktree`: Worktree path (empty for single-threaded)
- `error`: Error message if crashed

## LLM Backend

### Provider Interface

```go
type Provider interface {
    Propose(ctx context.Context, req ProposalRequest) (*Proposal, error)
}

type ProposalRequest struct {
    SystemPrompt   string
    FileContents   map[string]string  // filename -> content
    PastResults    []Result
    MetricName     string
    MetricDirection string  // "minimize" or "maximize"
    BestMetric     float64
    Constraints    []string
}

type Proposal struct {
    TargetFile  string
    Diff        string   // unified diff format
    Description string
    Reasoning   string
}
```

### System Prompt Template

The system prompt instructs the LLM to:
1. Analyze the current code and past experiment results
2. Identify one targeted, minimal change to try
3. Output a unified diff and a one-line description
4. Explain reasoning briefly
5. Avoid changes that have already been tried and failed
6. Prefer simple changes over complex ones

### Supported Backends

| Backend | Config | Authentication |
|---------|--------|---------------|
| Claude (Anthropic) | `--llm-backend claude --llm-model claude-sonnet-4-20250514` | `ANTHROPIC_API_KEY` env var |
| OpenAI-compatible | `--llm-backend openai --llm-model gpt-4o` | `OPENAI_API_KEY` env var, `OPENAI_BASE_URL` for custom endpoints |
| llama.cpp | `--llm-backend llamacpp --llm-model /path/to/model.gguf` | Local binary at `llama-cli` or `LLAMACPP_PATH` env var |

## Configuration File

`.autoresearch/config.yaml`:

```yaml
# autoresearch configuration
script: train.py
metric: val_bpb
direction: minimize          # minimize or maximize
branch: autoresearch/auto
run_command: "uv run"        # command prefix to run the script
target_file: train.py        # file the LLM modifies

# Execution
iterations: 0                # 0 = unlimited
threshold: 0.0               # minimum improvement to commit
parallel: 5                  # parallel experiment channels
timeout: 10m                 # max time per experiment

# LLM
llm_backend: claude
llm_model: ""                # empty = provider default
llm_temperature: 0.7         # higher = more diverse proposals

# Guard rails
max_consecutive_failures: 10 # halt after N consecutive failures
max_total_crashes: 20        # halt after N total crashes

# Metric parsing
metric_pattern: "^{metric}:\\s+([\\d.]+)"  # regex to extract metric, {metric} is replaced

# TUI
tui: true                    # enable TUI dashboard
```

## Guard Rails

| Guard Rail | Default | Flag | Behavior |
|-----------|---------|------|----------|
| Max iterations | unlimited | `--iterations/-i` | Halt loop after N iterations |
| Min threshold | 0.0 | `--threshold/-t` | Only commit if metric improves by >= T |
| Experiment timeout | 10m | `--timeout` | Kill experiment process after duration |
| Crash rollback | always | -- | `git reset --hard` on crash |
| Max consecutive failures | 10 | config only | Halt if 10 experiments fail in a row |
| Max total crashes | 20 | config only | Halt if 20 total crashes |
| Graceful shutdown | SIGINT/SIGTERM | -- | Finish current experiment, merge improvements, clean worktrees, exit |

## TUI Dashboard

Built with [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss).

Layout:
```
+------------------------------------------------------------------+
| autoresearch v0.1.0 | branch: autoresearch/mar29 | iter: 5/100   |
+------------------------------------------------------------------+
| Metric: val_bpb (minimize)  Best: 0.985234  Baseline: 0.997900  |
|                                                                   |
|  1.000 |*                                                         |
|  0.995 | *  *                                                     |
|  0.990 |  *   *  *                                                |
|  0.985 |         *                                                |
|        +------------ iteration                                    |
|                                                                   |
+------------------------------------------------------------------+
| Recent Experiments:                                               |
|  #5  keep     0.985234  "reduce depth to 6, increase batch"      |
|  #4  discard  0.998100  "switch to GELU activation"              |
|  #3  keep     0.990567  "increase LR to 0.06"                   |
|  #2  discard  1.002340  "add dropout 0.1"                        |
|  #1  keep     0.993200  "increase MATRIX_LR to 0.04"            |
+------------------------------------------------------------------+
| Active: 3/5 channels running | Keep rate: 60% | Elapsed: 25m     |
+------------------------------------------------------------------+
| [q] quit  [p] pause  [r] resume  [d] details                     |
+------------------------------------------------------------------+
```

## Dependencies (Go Modules)

```
github.com/spf13/cobra           # CLI framework
github.com/spf13/viper           # Configuration
github.com/charmbracelet/bubbletea  # TUI framework
github.com/charmbracelet/lipgloss   # TUI styling
github.com/charmbracelet/bubbles    # TUI components
github.com/sashabaranov/go-openai   # OpenAI API client (works for Claude too via adapter)
```

## Error Handling

- **LLM API failure**: Retry with exponential backoff (3 attempts), then skip iteration
- **Patch apply failure**: Log as "invalid_patch", request new proposal
- **Experiment crash**: `git reset --hard`, log as "crash", continue
- **Merge conflict**: Skip merge for this experiment, log as "conflict"
- **Git operation failure**: Fatal error, halt loop
- **Timeout exceeded**: Kill process, treat as crash
- **SIGINT/SIGTERM**: Finish current experiment, merge any improvements, clean up, exit with summary

## Metric Parsing

The metric parser scans experiment output line-by-line for a pattern matching:

```
<metric_name>:<whitespace><numeric_value>
```

Example for `val_bpb`:
```
val_bpb:          0.997900
```

Parsed as: metric_name="val_bpb", metric_value=0.997900

The regex pattern is configurable via `metric_pattern` in config. The default pattern:
```regex
^{metric}:\s+([\d.]+)
```

Where `{metric}` is replaced with the `--metric` flag value.

## Git Workflow

```
main (or master)
  |
  +-- autoresearch/<tag>     <- experiment branch
       |
       +-- commit: baseline (iter 0)
       +-- commit: "increase LR" (iter 1, keep)
       +-- commit: "reduce depth" (iter 3, keep)
       +-- ...
```

For parallel mode, worktree branches are:
```
autoresearch/<tag>
  |
  +-- autoresearch/<tag>/exp-2-ch0   (worktree, merged if improved)
  +-- autoresearch/<tag>/exp-2-ch1   (worktree, deleted if not improved)
  +-- autoresearch/<tag>/exp-2-ch2   (worktree, merged if improved)
  ...
```

## Graceful Shutdown Sequence

1. Receive SIGINT or SIGTERM
2. Set shutdown flag (atomic bool)
3. If experiments are running: wait for them to complete (up to timeout)
4. If any experiments improved: merge those worktrees
5. Clean up all remaining worktrees
6. Print final summary (total iterations, best metric, keep rate)
7. Exit 0

## Build and Install

```bash
cd /home/n0ko/Programs/autoresearch
go build -o autoresearch ./cmd/autoresearch
# or
go install ./cmd/autoresearch
```

## Example Usage

```bash
# Initialize config
autoresearch init

# Run with defaults (reads .autoresearch/config.yaml)
autoresearch run --script train.py --metric val_bpb

# Run with overrides
autoresearch run \
  --script train.py \
  --metric val_bpb \
  --branch autoresearch/mar29 \
  --iterations 100 \
  --threshold 0.001 \
  --parallel 3 \
  --timeout 7m \
  --llm-backend claude

# Check results
autoresearch results --last 20
autoresearch results --best
```
