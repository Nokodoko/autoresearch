// Package ob1 provides a read/append-only HTTP client for the OpenBrain REST API.
// It allows autoresearch to persist experiment results in OpenBrain's item store
// and retrieve past experiment history for richer LLM context.
//
// SAFETY: This package intentionally exposes NO delete, update, archive, or compact
// operations. Only read and append are supported.
package ob1

import "time"

// ExperimentEntry represents an autoresearch experiment result stored in OpenBrain.
type ExperimentEntry struct {
	Iteration   int     `json:"iteration"`
	Channel     int     `json:"channel,omitempty"`
	MetricName  string  `json:"metric_name"`
	MetricValue float64 `json:"metric_value"`
	BestMetric  float64 `json:"best_metric"`
	Status      string  `json:"status"` // keep, discard, crash, conflict
	Description string  `json:"description"`
	Commit      string  `json:"commit,omitempty"`
	Branch      string  `json:"branch,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// createEntryRequest is the JSON body for POST /api/v1/openbrain/entries.
type createEntryRequest struct {
	RawContent     string          `json:"raw_content"`
	RawContentType string          `json:"raw_content_type,omitempty"`
	ItemType       string          `json:"item_type,omitempty"`
	Priority       int             `json:"priority,omitempty"`
	Entities       entitiesPayload `json:"entities,omitempty"`
}

// entitiesPayload holds tags for the entities JSONB field.
type entitiesPayload struct {
	Tags []string `json:"tags,omitempty"`
}

// createEntryResponse is the JSON response from POST /api/v1/openbrain/entries.
type createEntryResponse struct {
	Success    bool   `json:"success"`
	ID         string `json:"id"`
	ItemType   string `json:"item_type"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	AuditID    string `json:"audit_id"`
	CapturedAt string `json:"captured_at"`
	Error      string `json:"error,omitempty"`
}

// listEntriesResponse is the JSON response from GET /api/v1/openbrain/entries.
type listEntriesResponse struct {
	Entries []entryItem `json:"entries"`
	Count   int         `json:"count"`
}

// entryItem is a single item returned from the entries list endpoint.
type entryItem struct {
	ID             string  `json:"id"`
	RawContent     string  `json:"raw_content"`
	ItemType       *string `json:"item_type"`
	Priority       *int    `json:"priority"`
	Status         string  `json:"status"`
	CapturedAt     string  `json:"captured_at"`
}
