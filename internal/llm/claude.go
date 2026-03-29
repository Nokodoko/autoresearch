package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	claudeAPIURL       = "https://api.anthropic.com/v1/messages"
	claudeDefaultModel = "claude-sonnet-4-20250514"
	claudeAPIVersion   = "2023-06-01"
)

// ClaudeProvider implements the Provider interface for Anthropic Claude.
type ClaudeProvider struct {
	apiKey      string
	model       string
	temperature float64
	client      *http.Client
}

// NewClaudeProvider creates a Claude LLM provider.
func NewClaudeProvider(model string, temperature float64) (*ClaudeProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required for claude backend")
	}
	if model == "" {
		model = claudeDefaultModel
	}
	return &ClaudeProvider{
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		client:      &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (c *ClaudeProvider) Name() string {
	return "claude"
}

// Propose sends a proposal request to Claude and parses the response.
func (c *ClaudeProvider) Propose(ctx context.Context, req ProposalRequest) (*Proposal, error) {
	systemPrompt := BuildSystemPrompt(req.MetricName, req.MetricDirection)
	userPrompt := BuildUserPrompt(req.FileContents, req.PastResults, req.BestMetric, req.MetricName, req.OB1History)

	body := map[string]any{
		"model":       c.model,
		"max_tokens":  8192,
		"temperature": c.temperature,
		"system":      systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", claudeAPIVersion)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return parseClaudeResponse(respBody)
}

// parseClaudeResponse extracts the Proposal from Claude's API response.
func parseClaudeResponse(body []byte) (*Proposal, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Claude response: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty Claude response")
	}

	// Find the text content
	text := ""
	for _, c := range resp.Content {
		if c.Type == "text" {
			text = c.Text
			break
		}
	}

	if text == "" {
		return nil, fmt.Errorf("no text in Claude response")
	}

	// Extract JSON from the response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(text)

	var proposal Proposal
	if err := json.Unmarshal([]byte(jsonStr), &proposal); err != nil {
		return nil, fmt.Errorf("parsing proposal JSON: %w\nRaw text: %s", err, text[:min(len(text), 500)])
	}

	if proposal.TargetFile == "" || proposal.NewContent == "" {
		return nil, fmt.Errorf("incomplete proposal: missing target_file or new_content")
	}

	return &proposal, nil
}

// extractJSON tries to find a JSON object in text, handling markdown code blocks.
func extractJSON(text string) string {
	// Try to find JSON in code blocks first
	start := -1
	for i := 0; i < len(text)-2; i++ {
		if text[i] == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return text
	}

	// Find matching closing brace
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		if escaped {
			escaped = false
			continue
		}
		if text[i] == '\\' && inString {
			escaped = true
			continue
		}
		if text[i] == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if text[i] == '{' {
			depth++
		} else if text[i] == '}' {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return text[start:]
}
