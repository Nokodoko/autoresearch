package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/n0ko/autoresearch/internal/config"
	"github.com/n0ko/autoresearch/internal/loop"
	"github.com/n0ko/autoresearch/internal/parallel"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)

	metricStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	keepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	discardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	crashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
)

// ResultEntry is a displayed result row.
type ResultEntry struct {
	Iteration   int
	Channel     int
	Status      string
	Metric      float64
	Description string
}

type model struct {
	cfg       *config.Config
	events    chan loop.Event
	results   []ResultEntry
	metrics   []float64 // metric values for chart
	bestMetric float64
	iteration  int
	status     string // "running", "proposing", "done"
	startTime  time.Time
	width      int
	height     int
	done       bool
	paused     bool
	err        error
}

type tickMsg time.Time
type eventMsg loop.Event

// App wraps the bubbletea application.
type App struct {
	cfg    *config.Config
	events chan loop.Event
}

// NewApp creates a TUI app.
func NewApp(cfg *config.Config, events chan loop.Event) *App {
	return &App{cfg: cfg, events: events}
}

// RunWithEngine starts the TUI alongside a single-threaded engine.
func (a *App) RunWithEngine(ctx context.Context, engine *loop.Engine) error {
	go func() {
		engine.Run(ctx)
	}()
	return a.run()
}

// RunWithOrchestrator starts the TUI alongside the parallel orchestrator.
func (a *App) RunWithOrchestrator(ctx context.Context, orch *parallel.Orchestrator) error {
	go func() {
		orch.Run(ctx)
	}()
	return a.run()
}

func (a *App) run() error {
	m := &model{
		cfg:       a.cfg,
		events:    a.events,
		startTime: time.Now(),
		width:     80,
		height:    24,
		status:    "initializing",
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		listenForEvents(m.events),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func listenForEvents(ch chan loop.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return eventMsg(loop.Event{Type: "done"})
		}
		return eventMsg(ev)
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "p":
			m.paused = !m.paused
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tickCmd()

	case eventMsg:
		ev := loop.Event(msg)
		switch ev.Type {
		case "iteration_start":
			m.iteration = ev.Iteration
			m.status = "proposing"
		case "proposal":
			m.status = "proposing"
		case "running":
			m.status = "running"
		case "result":
			m.results = append(m.results, ResultEntry{
				Iteration:   ev.Iteration,
				Channel:     ev.Channel,
				Status:      ev.Status,
				Metric:      ev.Metric,
				Description: ev.Description,
			})
			if ev.Metric > 0 {
				m.metrics = append(m.metrics, ev.Metric)
			}
			m.bestMetric = ev.BestMetric
		case "done":
			m.done = true
			m.status = "done"
			m.bestMetric = ev.BestMetric
			return m, tea.Quit
		case "error":
			m.err = fmt.Errorf("%s", ev.Error)
		}
		return m, listenForEvents(m.events)
	}

	return m, nil
}

func (m *model) View() string {
	var sb strings.Builder

	// Header
	elapsed := time.Since(m.startTime).Round(time.Second)
	iterStr := fmt.Sprintf("iter: %d", m.iteration)
	if m.cfg.Iterations > 0 {
		iterStr = fmt.Sprintf("iter: %d/%d", m.iteration, m.cfg.Iterations)
	}

	header := titleStyle.Render(fmt.Sprintf(
		" autoresearch v0.1.0 | branch: %s | %s | %s ",
		m.cfg.Branch, iterStr, elapsed,
	))
	sb.WriteString(header)
	sb.WriteString("\n\n")

	// Metric summary
	metricLine := fmt.Sprintf("Metric: %s (%s)  Best: %s",
		m.cfg.Metric, m.cfg.Direction,
		metricStyle.Render(fmt.Sprintf("%.6f", m.bestMetric)),
	)
	sb.WriteString(metricLine)
	sb.WriteString("\n\n")

	// Chart
	if len(m.metrics) > 1 {
		chart := renderChart(m.metrics, m.width-6, 8)
		sb.WriteString(borderStyle.Render(chart))
		sb.WriteString("\n\n")
	}

	// Recent results
	sb.WriteString("Recent Experiments:\n")
	start := 0
	maxShow := min(15, m.height-16)
	if len(m.results) > maxShow {
		start = len(m.results) - maxShow
	}
	for _, r := range m.results[start:] {
		var style lipgloss.Style
		switch r.Status {
		case "keep":
			style = keepStyle
		case "discard":
			style = discardStyle
		default:
			style = crashStyle
		}

		desc := r.Description
		maxDesc := m.width - 35
		if maxDesc > 0 && len(desc) > maxDesc {
			desc = desc[:maxDesc-3] + "..."
		}

		line := fmt.Sprintf("  #%-3d %-8s %.6f  %s",
			r.Iteration, style.Render(r.Status), r.Metric, desc)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Stats bar
	sb.WriteString("\n")
	kept := 0
	discarded := 0
	crashed := 0
	for _, r := range m.results {
		switch r.Status {
		case "keep":
			kept++
		case "discard":
			discarded++
		default:
			crashed++
		}
	}

	total := kept + discarded
	keepRate := 0.0
	if total > 0 {
		keepRate = float64(kept) / float64(total) * 100
	}

	statusText := m.status
	if m.paused {
		statusText = "PAUSED"
	}

	statsLine := fmt.Sprintf("Status: %s | Keep: %d | Discard: %d | Crash: %d | Rate: %.0f%% | Elapsed: %s",
		statusText, kept, discarded, crashed, keepRate, elapsed)
	sb.WriteString(statsLine)
	sb.WriteString("\n")

	// Controls
	sb.WriteString("[q] quit  [p] pause/resume\n")

	return sb.String()
}
