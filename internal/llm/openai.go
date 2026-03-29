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
	openaiDefaultURL   = "https://api.openai.com/v1/chat/completions"
	openaiDefaultModel = "gpt-4o"
)

// OpenAIProvider implements the Provider interface for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey      string
	baseURL     string
	model       string
	temperature float64
	client      *http.Client
}

// NewOpenAIProvider creates an OpenAI-compatible LLM provider.
func NewOpenAIProvider(model string, temperature float64) (*OpenAIProvider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for openai backend")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = openaiDefaultURL
	}
	if model == "" {
		model = openaiDefaultModel
	}
	return &OpenAIProvider{
		apiKey:      apiKey,
		baseURL:     baseURL,
		model:       model,
		temperature: temperature,
		client:      &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

// Propose sends a proposal request to the OpenAI-compatible API.
func (o *OpenAIProvider) Propose(ctx context.Context, req ProposalRequest) (*Proposal, error) {
	systemPrompt := BuildSystemPrompt(req.MetricName, req.MetricDirection)
	userPrompt := BuildUserPrompt(req.FileContents, req.PastResults, req.BestMetric, req.MetricName)

	body := map[string]any{
		"model":       o.model,
		"max_tokens":  8192,
		"temperature": o.temperature,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return parseOpenAIResponse(respBody)
}

func parseOpenAIResponse(body []byte) (*Proposal, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing OpenAI response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty OpenAI response")
	}

	text := resp.Choices[0].Message.Content
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
