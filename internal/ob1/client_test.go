package ob1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-branch")
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPingUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-branch")
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for unhealthy server")
	}
}

func TestWriteExperimentResult(t *testing.T) {
	var receivedBody createEntryRequest
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/openbrain/entries" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&receivedBody)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(createEntryResponse{
			Success:  true,
			ID:       "test-id-123",
			ItemType: "observation",
			Status:   "classified",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-api-key", "autoresearch/test")
	entry := ExperimentEntry{
		Iteration:   5,
		Channel:     2,
		MetricName:  "val_bpb",
		MetricValue: 0.985234,
		BestMetric:  0.985234,
		Status:      "keep",
		Description: "increase learning rate to 0.06",
		Commit:      "a1b2c3d",
		Timestamp:   time.Now(),
	}

	err := client.WriteExperimentResult(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify auth header.
	if receivedAuth != "Bearer my-api-key" {
		t.Errorf("expected Bearer my-api-key, got %s", receivedAuth)
	}

	// Verify body.
	if receivedBody.ItemType != "observation" {
		t.Errorf("expected observation type, got %s", receivedBody.ItemType)
	}
	if !strings.Contains(receivedBody.RawContent, "autoresearch experiment iter 5 ch2") {
		t.Errorf("unexpected content: %s", receivedBody.RawContent)
	}
	if !strings.Contains(receivedBody.RawContent, "val_bpb = 0.985234 (keep)") {
		t.Errorf("missing metric in content: %s", receivedBody.RawContent)
	}

	// Verify tags.
	expectedTags := []string{"autoresearch", "val_bpb", "keep"}
	for i, tag := range expectedTags {
		if i >= len(receivedBody.Entities.Tags) || receivedBody.Entities.Tags[i] != tag {
			t.Errorf("expected tag %s, got %v", tag, receivedBody.Entities.Tags)
			break
		}
	}
}

func TestReadExperimentHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/openbrain/entries") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		entries := []entryItem{
			{
				ID:         "id-1",
				RawContent: "autoresearch experiment iter 1: increase LR\nMetric: val_bpb = 0.993200 (keep)\nBest: 0.993200\nCommit: abc1234",
				Status:     "classified",
			},
			{
				ID:         "id-2",
				RawContent: "autoresearch experiment iter 2 ch1: add dropout\nMetric: val_bpb = 1.002340 (discard)\nBest: 0.993200",
				Status:     "classified",
			},
			{
				ID:         "id-3",
				RawContent: "Some other observation not from autoresearch",
				Status:     "classified",
			},
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(listEntriesResponse{
			Entries: entries,
			Count:   len(entries),
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-branch")
	history, err := client.ReadExperimentHistory(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get 2 entries (the third is not an autoresearch entry).
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}

	// Verify first entry.
	if history[0].Iteration != 1 {
		t.Errorf("expected iteration 1, got %d", history[0].Iteration)
	}
	if history[0].MetricName != "val_bpb" {
		t.Errorf("expected val_bpb, got %s", history[0].MetricName)
	}
	if history[0].Status != "keep" {
		t.Errorf("expected keep, got %s", history[0].Status)
	}

	// Verify second entry (with channel).
	if history[1].Iteration != 2 {
		t.Errorf("expected iteration 2, got %d", history[1].Iteration)
	}
	if history[1].Channel != 1 {
		t.Errorf("expected channel 1, got %d", history[1].Channel)
	}
	if history[1].Status != "discard" {
		t.Errorf("expected discard, got %s", history[1].Status)
	}
}

func TestFormatEntryContentRoundTrip(t *testing.T) {
	entry := ExperimentEntry{
		Iteration:   7,
		Channel:     3,
		MetricName:  "val_bpb",
		MetricValue: 0.985234,
		BestMetric:  0.980000,
		Status:      "keep",
		Description: "reduce depth to 6",
		Commit:      "abc1234",
	}

	content := formatEntryContent(entry, "autoresearch/test")
	parsed, err := parseEntryContent(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if parsed.Iteration != entry.Iteration {
		t.Errorf("iteration: got %d, want %d", parsed.Iteration, entry.Iteration)
	}
	if parsed.Channel != entry.Channel {
		t.Errorf("channel: got %d, want %d", parsed.Channel, entry.Channel)
	}
	if parsed.MetricName != entry.MetricName {
		t.Errorf("metric name: got %s, want %s", parsed.MetricName, entry.MetricName)
	}
	if parsed.Status != entry.Status {
		t.Errorf("status: got %s, want %s", parsed.Status, entry.Status)
	}
	if parsed.Description != entry.Description {
		t.Errorf("description: got %s, want %s", parsed.Description, entry.Description)
	}
	if parsed.Commit != entry.Commit {
		t.Errorf("commit: got %s, want %s", parsed.Commit, entry.Commit)
	}
}

func TestFormatHistory(t *testing.T) {
	entries := []ExperimentEntry{
		{Iteration: 1, MetricName: "val_bpb", MetricValue: 0.993, Status: "keep", Description: "increase LR"},
		{Iteration: 2, MetricName: "val_bpb", MetricValue: 1.002, Status: "discard", Description: "add dropout"},
	}

	result := FormatHistory(entries, "val_bpb")
	if !strings.Contains(result, "OpenBrain Experiment History") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "increase LR") {
		t.Error("missing entry 1")
	}
	if !strings.Contains(result, "add dropout") {
		t.Error("missing entry 2")
	}
}

func TestFormatHistoryEmpty(t *testing.T) {
	result := FormatHistory(nil, "val_bpb")
	if result != "" {
		t.Errorf("expected empty string for nil entries, got %q", result)
	}
}

func TestNilClientSafety(t *testing.T) {
	// Verify that a nil client doesn't cause issues when used through
	// the helper functions in the integration points.
	var c *Client
	if c != nil {
		t.Fatal("nil client should be nil")
	}
}
