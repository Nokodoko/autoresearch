/sr

# Feature Request: "autoresearch" — Self-Improving LLM Training Loop

## Summary

Create a 'autoresearch' tool that implements a self-improving training loop for LLMs. The agent iteratively proposes code changes, runs experiments, and commits improvements to git. This enables autonomous optimization of model performance over time.

## Implementation Details

<!-- Technical breakdown: file paths, code changes, config, integration points -->

1. See the outline below beginning with # Source

###

## Considerations

<!-- Edge cases, trade-offs, alternatives, dependencies, open questions -->

1. The idea is that this is go be a generalized framework on how to train an agent, or skill, or command, any type of intelligence primative on how to improve itself. The core loop is to read the current state of the codebase, propose a change, run an experiment, and then commit or revert based on the results. This can be applied to any iterative optimization problem, not just LLM training.
2. This will need clear 'evals' or 'metrics' to optimize for, and a way to parse those metrics from the experiment runs. The agent will need to be able to understand the results and make informed decisions about whether to commit or revert changes. This should be treated like "# Outputs" is in the 'intent-manager' spec, where we have a single scalar metric that we are trying to optimize, and the agent's job is to propose changes that improve that metric.
3. Use the python files found in this directory as scaffolding for the 'autoresearch' command. To make this our own and really improve development speed, convert all scripts into golang. Then use channels to spawn 'go run' processes to execute experiments in parrallel [this should also be a configurable option, as some experiments may not be parallelizable or may require more resources than we have available]. The agent loop will read the git log and results log to understand the current state of the codebase and past experiment results, then call the LLM to propose the next edit, apply the patch, run the script, parse the metric, and commit or revert based on the results., via a '--iterations' or '-i' flag to specify how many iterations to run before halting, and a '--threshold' or '-t' flag to specify a minimum improvement threshold for committing changes. The results log will be an append-only JSONL file at '.autoresearch/results.jsonl' that contains the timestamp, commit hash, metric, and edit summary for each experiment run. The LLM backend will be configurable, allowing users to choose between different APIs or local models. We will also implement guard rails to prevent runaway experiments, such as a maximum iteration cap, a minimum improvement threshold, and automatic rollback on crashes. Finally, we will build a TUI dashboard that provides a live view of the experiment progress, including the metric curve and recent commits.]

   Each go routine will open a separate worktree. Upon completion of the run all worktrees with improvements will be merged back into the main branch, and all worktrees with no improvement will be deleted. This allows us to run multiple experiments in parallel without worrying about conflicts, and also allows us to easily track which experiments led to improvements and which did not. The number of parallel experiments can be configured via a '--parallel' or '-p' flag, which specifies how many channels to use for running experiments in parallel.

   3a. The default value will be 5 channels per loop, and the scores from each channel will feed back into the main thread, where any thread with an improvement, will have the change committed, and merged.

4. Again this is designed to be used with any iterative optimization problem, not just LLM training. The core loop of reading the current state, proposing a change, running an experiment, and committing or reverting based on the results can be applied to any problem where you have a clear metric to optimize for and a way to propose changes to the codebase. This makes it a versatile framework for autonomous optimization tasks.

## Workflow

<!-- Describe the step-by-step user experience as numbered actions -->

1.

# autoresearch — Self-Improving LLM Training Loop

## Source

Andrej Karpathy (@karpathy) — self-contained minimal repo, ~630 lines, single-GPU nanochatLM training core.
Originally built for self-improving LLMs but the framework applies to any iterative optimization problem.

## Core Loop

1. **Read context** — agent reads current codebase state + all previous experiment results
2. **Propose edit** — agent proposes a targeted, minimal code change (one hypothesis at a time)
3. **Run experiment** — execute a fast, reproducible training/eval run
4. **Score** — extract a single objective scalar metric (loss, accuracy, throughput, etc.)
5. **Commit or revert** — git-commit if score improves; revert if it doesn't
6. **Repeat** — loop indefinitely on a dedicated feature branch

## Build Target

Implement autoresearch as a Go orchestrator wrapping any experiment script:

- `autoresearch run --script train.py --metric val_loss --branch experiments/auto`
- Agent loop: read git log + results log → call LLM for next edit → apply patch → run script → parse metric → commit or revert
- Results log: append-only JSONL at `.autoresearch/results.jsonl` — timestamp, commit hash, metric, edit summary
- LLM backend: configurable (Claude API, local llama.cpp, openai-compatible)
- Guard rails: max iterations, min improvement threshold, rollback on crash
- TUI dashboard: live view of experiment progress, metric curve, recent commits

## Key Design Constraints

- One change per iteration — no batched edits, keeps history clean and attribution clear
- Fast experiments only — if a run takes >N minutes, autoresearch skips it (configurable ceiling)
- Git-native — all state lives in git history; no external DB required
- Reproducible — each experiment run must be deterministic given the same seed/config

## Outcome

- `autoresearch run` executes the full loop autonomously
- Results are git-committed with descriptive messages
- Metric improves over baseline within 10 iterations on a toy benchmark
- Loop halts gracefully on threshold, iteration cap, or SIGINT

# Outcomes

1. The 'autoresearch' tool is implemented as a Go command-line application that orchestrates the self-improving training loop for LLMs. It reads the current state of the codebase and past experiment results, proposes code changes, runs experiments, parses metrics, and commits improvements to git based on the results. The tool supports parallel experiment execution using Go channels and provides configurable options for iteration limits, improvement thresholds, and parallelism. A TUI dashboard is also implemented to visualize experiment progress and metric curves in real-time. This should be invocable as a command with '/autoresearch' or '/ar' as an alias.
2. The 'autoresearch' tool successfully optimizes a toy benchmark within 10 iterations, demonstrating the effectiveness of the self-improving training loop. The results are git-committed with descriptive messages, and the loop halts gracefully on reaching the improvement threshold, iteration cap, or receiving a SIGINT signal. The tool is designed to be generalizable and can be applied to any iterative optimization problem, not just LLM training, making it a versatile framework for autonomous optimization tasks.
3. The 'autoresearch' tool is well-documented, with clear instructions on how to set up and use the command-line application. The codebase is organized and maintainable, following best practices for Go development. The tool is tested with various configurations and edge cases to ensure robustness and reliability in different scenarios. Overall, the 'autoresearch' tool provides a powerful framework for self-improving training loops, enabling users to optimize their models autonomously over time.
