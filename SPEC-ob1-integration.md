# SPEC: OpenBrain (ob1) Integration for autoresearch

## Summary

Add an ob1 connection to autoresearch so each optimization iteration can READ past experiment history from OpenBrain and WRITE new experiment results to OpenBrain. This gives the LLM proposer richer context about experiment history beyond the local JSONL log, and persists experiment knowledge in the organizational memory system.

## Architecture Decision

**Use ob1's REST API over HTTP** (`/api/v1/openbrain/entries`), not direct PostgreSQL access or MCP tool calls.

Rationale:
- autoresearch is a separate Go binary; HTTP is the cleanest boundary
- ob1's MCP server already exposes REST endpoints with auth
- No dependency on ob1's internal Go packages (different module, different go.mod)
- ob1 handles embedding generation, audit entries, and pipeline integration automatically
- Concurrency-safe: HTTP is inherently safe for concurrent goroutine access

## Integration Points

### 1. New Package: `internal/ob1/`

```
internal/ob1/
  client.go      -- HTTP client for ob1 REST API
  types.go       -- Request/response types
  history.go     -- History formatting for LLM context
```

### 2. `client.go` -- OB1 Client

```go
package ob1

// Client is an HTTP client for OpenBrain's REST API.
// It is concurrency-safe (http.Client is safe for concurrent use).
// CRITICAL: This client supports READ and APPEND operations ONLY.
// No delete, update, archive, or compact operations are exposed.
type Client struct {
    baseURL    string       // e.g., "http://localhost:8200"
    apiKey     string       // Bearer token for auth
    httpClient *http.Client
}

// NewClient creates an ob1 REST client.
func NewClient(baseURL, apiKey string) *Client

// WriteExperimentResult appends an experiment result as an ob1 item.
// Item type: "observation"
// Source: "mcp" (matches ob1's source enum)
// Tags: ["autoresearch", "<metric-name>", "<status>"]
// Content: structured text with iteration, metric, description, status
func (c *Client) WriteExperimentResult(ctx context.Context, result ExperimentEntry) error

// ReadExperimentHistory retrieves past autoresearch experiment items from ob1.
// Filters by type="observation" and searches for autoresearch-tagged items.
// Returns items ordered by captured_at DESC.
func (c *Client) ReadExperimentHistory(ctx context.Context, limit int) ([]ExperimentEntry, error)

// Ping checks if ob1 is reachable. Returns nil if healthy.
func (c *Client) Ping(ctx context.Context) error
```

### 3. `types.go` -- Data Types

```go
package ob1

// ExperimentEntry represents an experiment result stored in ob1.
type ExperimentEntry struct {
    Iteration   int     `json:"iteration"`
    Channel     int     `json:"channel"`
    MetricName  string  `json:"metric_name"`
    MetricValue float64 `json:"metric_value"`
    BestMetric  float64 `json:"best_metric"`
    Status      string  `json:"status"`      // keep, discard, crash
    Description string  `json:"description"`
    Commit      string  `json:"commit"`
    Branch      string  `json:"branch"`
    Timestamp   string  `json:"timestamp"`
}
```

### 4. `history.go` -- History Formatting

```go
package ob1

// FormatHistory formats ob1 experiment entries for LLM context.
// Returns a markdown-formatted string suitable for inclusion in the user prompt.
func FormatHistory(entries []ExperimentEntry, metricName string) string
```

### 5. Wire-up Points

#### a. Config (`internal/config/config.go`)

Add fields:
```go
// OpenBrain integration
OB1URL    string `mapstructure:"ob1_url"`    // default: "" (disabled)
OB1APIKey string `mapstructure:"ob1_api_key"` // from env AUTORESEARCH_OB1_API_KEY
```

When `OB1URL` is empty, the integration is disabled (no-op). This makes it backward-compatible.

#### b. Context Building (`internal/loop/context.go`)

Modify `BuildContext()` to accept an optional `*ob1.Client`. If non-nil, fetch experiment history from ob1 and add it to the Context struct.

Add field to `Context`:
```go
OB1History string // Formatted ob1 experiment history
```

#### c. LLM Prompt (`internal/llm/prompt.go`)

Modify `BuildUserPrompt()` to accept an optional `ob1History string` parameter. If non-empty, add a "## OpenBrain Experiment History" section to the prompt, placed between "Past Experiment Results" and the final instruction.

#### d. Engine (`internal/loop/engine.go`)

- Add `ob1Client *ob1.Client` field to Engine struct
- In `NewEngine()`, accept optional ob1 client
- In `runIteration()`, after the keep/discard decision and result logging, call `ob1Client.WriteExperimentResult()` if client is non-nil
- In `BuildContext()` calls, pass the ob1 client

#### e. Orchestrator (`internal/parallel/orchestrator.go`)

- Add `ob1Client *ob1.Client` field
- In `NewOrchestrator()`, accept optional ob1 client
- After processing channel results (keep/discard/crash), write each to ob1
- When building context for proposals, include ob1 history

#### f. Main (`cmd/autoresearch/main.go`)

- If `OB1URL` is configured, create `ob1.Client` and pass to Engine/Orchestrator
- Ping ob1 at startup; warn (don't fail) if unreachable
- Log ob1 connection status

## Append-Only Safety

The `ob1.Client` type MUST NOT expose any method that can:
- Delete items
- Update existing items
- Archive or compact items
- Call cleanup operations

The client struct exposes exactly two data operations: `WriteExperimentResult` (creates a new item) and `ReadExperimentHistory` (lists items). This is enforced at the Go type level -- the methods simply don't exist.

## Concurrency Safety

- `http.Client` is safe for concurrent use by multiple goroutines (Go stdlib guarantee)
- Each goroutine in the parallel orchestrator can call `WriteExperimentResult` independently
- No shared mutable state in the ob1 client
- ob1 server handles concurrent writes via PostgreSQL SERIALIZABLE transactions

## ob1 Item Format

When writing to ob1, each experiment result is stored as:

```json
{
    "raw_content": "autoresearch experiment iter 5 ch2: Increase learning rate to 0.06\nMetric: val_bpb = 0.985234 (keep)\nBest: 0.985234\nBranch: autoresearch/mar29\nCommit: a1b2c3d",
    "raw_content_type": "text/plain",
    "item_type": "observation",
    "priority": 2,
    "entities": {
        "tags": ["autoresearch", "val_bpb", "keep"]
    }
}
```

When reading from ob1, filter by tags containing "autoresearch" and parse the structured content.

## REST API Details

### Write (POST /api/v1/openbrain/entries)

Request:
```http
POST /api/v1/openbrain/entries HTTP/1.1
Host: localhost:8200
Authorization: Bearer <api_key>
Content-Type: application/json

{
    "raw_content": "...",
    "item_type": "observation",
    "priority": 2,
    "entities": {"tags": ["autoresearch", "val_bpb", "keep"]}
}
```

Response (201):
```json
{
    "success": true,
    "id": "018e...",
    "item_type": "observation",
    "status": "classified",
    "source": "mcp",
    "audit_id": "018e...",
    "captured_at": "2026-03-29T14:30:00Z"
}
```

### Read (GET /api/v1/openbrain/entries)

Request:
```http
GET /api/v1/openbrain/entries?type=observation&limit=50 HTTP/1.1
Host: localhost:8200
Authorization: Bearer <api_key>
```

Response:
```json
{
    "entries": [
        {
            "id": "...",
            "raw_content": "autoresearch experiment ...",
            "item_type": "observation",
            "status": "classified",
            "captured_at": "2026-03-29T14:30:00Z",
            ...
        }
    ],
    "count": 50
}
```

## Configuration

### Config file (`.autoresearch/config.yaml`)

```yaml
# OpenBrain integration (optional)
ob1_url: "http://localhost:8200"     # empty = disabled
ob1_api_key: ""                       # or set AUTORESEARCH_OB1_API_KEY env var
```

### Environment variables

```bash
AUTORESEARCH_OB1_URL=http://localhost:8200
AUTORESEARCH_OB1_API_KEY=mykey123
```

## Error Handling

- ob1 connection failure at startup: log warning, continue without ob1
- ob1 write failure during iteration: log warning, do NOT fail the iteration
- ob1 read failure during context building: log warning, proceed with local results only
- ob1 unreachable: graceful degradation, autoresearch works exactly as before

ob1 integration is best-effort. It must NEVER block or fail the core optimization loop.

## Testing

1. Unit tests for `internal/ob1/client.go` using httptest.Server
2. Unit tests for history formatting
3. Integration point: verify Engine/Orchestrator work with nil ob1 client (backward compat)
4. Integration point: verify Engine/Orchestrator work with ob1 client (mock HTTP)

## Files to Create

1. `internal/ob1/client.go` -- HTTP client
2. `internal/ob1/types.go` -- Types
3. `internal/ob1/history.go` -- History formatting
4. `internal/ob1/client_test.go` -- Tests

## Files to Modify

1. `internal/config/config.go` -- Add OB1URL, OB1APIKey fields
2. `internal/config/defaults.go` -- Add default values
3. `internal/loop/context.go` -- Add ob1 history to Context
4. `internal/loop/engine.go` -- Wire ob1 client, write results
5. `internal/parallel/orchestrator.go` -- Wire ob1 client, write results
6. `internal/llm/prompt.go` -- Add ob1 history section to prompt
7. `cmd/autoresearch/main.go` -- Initialize ob1 client
