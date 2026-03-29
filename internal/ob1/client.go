package ob1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for OpenBrain's REST API.
// It is concurrency-safe: http.Client is safe for concurrent use by multiple goroutines.
//
// SAFETY: This client supports READ and APPEND operations ONLY.
// No delete, update, archive, or compact operations are exposed.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	branch     string // current experiment branch name, included in writes
}

// NewClient creates an ob1 REST client.
// baseURL is the ob1 server address (e.g., "http://localhost:8200").
// apiKey is the Bearer token for authentication.
// branch is the current experiment branch name (included in written entries).
func NewClient(baseURL, apiKey, branch string) *Client {
	// Trim trailing slash from baseURL.
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		branch:  branch,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ping checks if ob1 is reachable by hitting the healthz endpoint.
// Returns nil if healthy, an error otherwise.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ob1 unreachable: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ob1 unhealthy: HTTP %d", resp.StatusCode)
	}
	return nil
}

// WriteExperimentResult appends an experiment result as an ob1 item.
// The item is created with type "observation" and tagged with "autoresearch".
// This is an append-only operation; no existing items are modified or deleted.
func (c *Client) WriteExperimentResult(ctx context.Context, entry ExperimentEntry) error {
	// Build content string with structured experiment data.
	content := formatEntryContent(entry, c.branch)

	// Build tags for filtering.
	tags := []string{"autoresearch", entry.MetricName, entry.Status}

	body := createEntryRequest{
		RawContent:     content,
		RawContentType: "text/plain",
		ItemType:       "observation",
		Priority:       2,
		Entities: entitiesPayload{
			Tags: tags,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling entry: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/openbrain/entries", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating write request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("writing to ob1: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ob1 write failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result createEntryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Write succeeded (2xx) but response parsing failed -- not critical.
		log.Printf("ob1: write succeeded but response parse failed: %v", err)
		return nil
	}

	if !result.Success {
		return fmt.Errorf("ob1 write failed: %s", result.Error)
	}

	return nil
}

// ReadExperimentHistory retrieves past autoresearch experiment items from ob1.
// Items are filtered by type "observation" and returned ordered by captured_at DESC.
// Only items whose content starts with "autoresearch experiment" are included.
func (c *Client) ReadExperimentHistory(ctx context.Context, limit int) ([]ExperimentEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	url := fmt.Sprintf("%s/api/v1/openbrain/entries?type=observation&limit=%d", c.baseURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating read request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading from ob1: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ob1 read failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result listEntriesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing ob1 response: %w", err)
	}

	// Filter and parse entries that are autoresearch experiments.
	var entries []ExperimentEntry
	for _, item := range result.Entries {
		if !strings.HasPrefix(item.RawContent, "autoresearch experiment") {
			continue
		}
		entry, err := parseEntryContent(item.RawContent)
		if err != nil {
			// Skip unparseable entries.
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// formatEntryContent formats an ExperimentEntry into the structured text content
// stored in ob1. The format is designed to be human-readable and machine-parseable.
func formatEntryContent(e ExperimentEntry, branch string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("autoresearch experiment iter %d", e.Iteration))
	if e.Channel > 0 {
		sb.WriteString(fmt.Sprintf(" ch%d", e.Channel))
	}
	sb.WriteString(fmt.Sprintf(": %s\n", e.Description))
	sb.WriteString(fmt.Sprintf("Metric: %s = %.6f (%s)\n", e.MetricName, e.MetricValue, e.Status))
	sb.WriteString(fmt.Sprintf("Best: %.6f\n", e.BestMetric))
	if branch != "" {
		sb.WriteString(fmt.Sprintf("Branch: %s\n", branch))
	}
	if e.Commit != "" {
		sb.WriteString(fmt.Sprintf("Commit: %s\n", e.Commit))
	}
	return sb.String()
}

// parseEntryContent parses the structured text content from an ob1 item back into
// an ExperimentEntry. Returns an error if the content cannot be parsed.
func parseEntryContent(content string) (ExperimentEntry, error) {
	var entry ExperimentEntry
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return entry, fmt.Errorf("too few lines")
	}

	// Parse first line: "autoresearch experiment iter N [chM]: description"
	firstLine := lines[0]
	if !strings.HasPrefix(firstLine, "autoresearch experiment iter ") {
		return entry, fmt.Errorf("unexpected format")
	}

	rest := strings.TrimPrefix(firstLine, "autoresearch experiment iter ")
	// Parse iteration number.
	idx := strings.IndexAny(rest, " :")
	if idx < 0 {
		return entry, fmt.Errorf("no iteration delimiter")
	}
	fmt.Sscanf(rest[:idx], "%d", &entry.Iteration)

	rest = rest[idx:]
	// Check for channel.
	if strings.HasPrefix(rest, " ch") {
		chPart := strings.TrimPrefix(rest, " ch")
		chIdx := strings.Index(chPart, ":")
		if chIdx >= 0 {
			fmt.Sscanf(chPart[:chIdx], "%d", &entry.Channel)
			rest = chPart[chIdx:]
		}
	}
	// Parse description after ": ".
	if colonIdx := strings.Index(rest, ": "); colonIdx >= 0 {
		entry.Description = strings.TrimSpace(rest[colonIdx+2:])
	}

	// Parse remaining lines.
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Metric: ") {
			metricPart := strings.TrimPrefix(line, "Metric: ")
			// Format: "metric_name = value (status)"
			eqIdx := strings.Index(metricPart, " = ")
			if eqIdx >= 0 {
				entry.MetricName = metricPart[:eqIdx]
				valuePart := metricPart[eqIdx+3:]
				parenIdx := strings.Index(valuePart, " (")
				if parenIdx >= 0 {
					fmt.Sscanf(valuePart[:parenIdx], "%f", &entry.MetricValue)
					statusPart := valuePart[parenIdx+2:]
					entry.Status = strings.TrimSuffix(statusPart, ")")
				} else {
					fmt.Sscanf(valuePart, "%f", &entry.MetricValue)
				}
			}
		} else if strings.HasPrefix(line, "Best: ") {
			fmt.Sscanf(strings.TrimPrefix(line, "Best: "), "%f", &entry.BestMetric)
		} else if strings.HasPrefix(line, "Commit: ") {
			entry.Commit = strings.TrimPrefix(line, "Commit: ")
		}
	}

	return entry, nil
}
