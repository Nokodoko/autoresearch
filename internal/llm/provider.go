package llm

import (
	"context"
	"fmt"

	"github.com/n0ko/autoresearch/internal/config"
	"github.com/n0ko/autoresearch/internal/results"
)

// ProposalRequest contains all context for the LLM to generate a proposal.
type ProposalRequest struct {
	SystemPrompt    string
	FileContents    map[string]string // filename -> content
	PastResults     []results.Result
	MetricName      string
	MetricDirection string
	BestMetric      float64
	Constraints     []string
	OB1History      string // Pre-formatted OpenBrain history section (empty if disabled)
}

// Proposal is the LLM's suggested code change.
type Proposal struct {
	TargetFile  string `json:"target_file"`
	NewContent  string `json:"new_content"`  // Complete new file content
	Description string `json:"description"`
	Reasoning   string `json:"reasoning"`
}

// Provider is the interface for LLM backends.
type Provider interface {
	Propose(ctx context.Context, req ProposalRequest) (*Proposal, error)
	Name() string
}

// NewProvider creates an LLM provider based on configuration.
func NewProvider(cfg *config.Config) (Provider, error) {
	switch cfg.LLMBackend {
	case "claude":
		return NewClaudeProvider(cfg.LLMModel, cfg.LLMTemperature)
	case "openai":
		return NewOpenAIProvider(cfg.LLMModel, cfg.LLMTemperature)
	case "llamacpp":
		return NewLlamaCppProvider(cfg.LLMModel, cfg.LLMTemperature)
	default:
		return nil, fmt.Errorf("unknown LLM backend: %s", cfg.LLMBackend)
	}
}
