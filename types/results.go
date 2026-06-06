package types

import "fmt"

// TaskResult is the result of a task execution.
type TaskResult struct {
	Output          map[string]any `json:"output"`
	Summary         string         `json:"summary"`
	UsedTools       []string       `json:"used_tools,omitempty"`
	DownstreamCalls []string       `json:"downstream_calls,omitempty"`
	Success         bool           `json:"success"`
	Error           string         `json:"error,omitempty"`
}

// SuccessResult builds a successful TaskResult.
func SuccessResult(output map[string]any, summary string, usedTools []string) *TaskResult {
	if summary == "" {
		summary = fmt.Sprintf("%v", output)
	}
	if usedTools == nil {
		usedTools = []string{}
	}
	return &TaskResult{Output: output, Summary: summary, UsedTools: usedTools, Success: true}
}

// ErrorResult builds a failed TaskResult.
func ErrorResult(err string) *TaskResult {
	return &TaskResult{
		Output:  map[string]any{"error": err},
		Summary: err,
		Success: false,
		Error:   err,
	}
}

// Event type strings emitted by the AIP platform run stream. The platform uses
// both dotted and underscored variants for the run/orchestrator lifecycle, so
// both are recognized here (kept in sync with the Python SDK).
const (
	EventRunCompleted                    = "run.completed"
	EventRunCompletedUnderscore          = "run_completed"
	EventOrchestratorCompleted           = "orchestrator.completed"
	EventOrchestratorCompletedUnderscore = "orchestrator_completed"
	EventRunFailed                       = "run.failed"
	EventOrchestratorError               = "orchestrator.error"
	EventError                           = "error"
	EventAgentInvoked                    = "agent_invoked"
	EventAgentCompleted                  = "agent_completed"
	EventPaymentSettled                  = "payment.settled"
	EventMemoryUploaded                  = "memory_uploaded"
)

// completedEventTypes and errorEventTypes are the canonical terminal-event sets.
var (
	completedEventTypes = map[string]bool{
		EventRunCompleted:                    true,
		EventRunCompletedUnderscore:          true,
		EventOrchestratorCompleted:           true,
		EventOrchestratorCompletedUnderscore: true,
	}
	errorEventTypes = map[string]bool{
		EventRunFailed:         true,
		EventOrchestratorError: true,
		EventError:             true,
	}
)

// EventData is data from a streaming run event.
type EventData struct {
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	Timestamp string         `json:"timestamp,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
}

// IsCompleted reports whether the event indicates completion.
func (e EventData) IsCompleted() bool { return completedEventTypes[e.EventType] }

// IsError reports whether the event indicates an error.
func (e EventData) IsError() bool { return errorEventTypes[e.EventType] }

// Message returns the event message, if present in the payload.
func (e EventData) Message() string {
	if m, ok := e.Payload["message"].(string); ok && m != "" {
		return m
	}
	if s, ok := e.Payload["summary"].(string); ok {
		return s
	}
	return ""
}

// RunResult is the result of running a task through the orchestrator.
type RunResult struct {
	RunID    string           `json:"run_id"`
	Status   RunStatus        `json:"status"`
	Result   map[string]any   `json:"result,omitempty"`
	Events   []EventData      `json:"events,omitempty"`
	Error    string           `json:"error,omitempty"`
	Payments []map[string]any `json:"payments,omitempty"`
}

// Success reports whether the run completed successfully.
func (r RunResult) Success() bool { return r.Status == RunStatusCompleted }

// Output returns the main output from the result, if any.
func (r RunResult) Output() any {
	if r.Result == nil {
		return nil
	}
	if v, ok := r.Result["output"]; ok && v != nil {
		return v
	}
	return r.Result["result"]
}

// AgentInfo describes a registered agent.
type AgentInfo struct {
	AgentID         string           `json:"agent_id"`
	Handle          string           `json:"handle"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	Capabilities    []string         `json:"capabilities,omitempty"`
	Skills          []map[string]any `json:"skills,omitempty"`
	Price           float64          `json:"price"`
	EndpointURL     string           `json:"endpoint_url,omitempty"`
	OnChain         bool             `json:"on_chain"`
	IdentityAddress string           `json:"identity_address,omitempty"`
}

// AgentInfoFromMap builds an AgentInfo from an API response map, matching the
// nesting and fallbacks of the Python AgentInfo.from_dict.
func AgentInfoFromMap(data map[string]any) AgentInfo {
	getStr := func(m map[string]any, k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	card, _ := data["card"].(map[string]any)
	if card == nil {
		card = map[string]any{}
	}

	var price float64
	if pd, ok := data["price"].(map[string]any); ok {
		if a, ok := pd["amount"].(float64); ok {
			price = a
		}
	}

	var capabilities []string
	switch raw := card["capabilities"].(type) {
	case map[string]any:
		for k, v := range raw {
			if b, ok := v.(bool); ok && b {
				capabilities = append(capabilities, k)
			}
		}
	case []any:
		for _, item := range raw {
			if s, ok := item.(string); ok {
				capabilities = append(capabilities, s)
			}
		}
	case []string:
		capabilities = raw
	}

	metadata, _ := data["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	onChain, _ := metadata["onchain"].(bool)

	name := getStr(card, "name")
	if name == "" {
		name = getStr(data, "name")
	}
	desc := getStr(card, "description")
	if desc == "" {
		desc = getStr(data, "description")
	}

	var skills []map[string]any
	if raw, ok := data["skills"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				skills = append(skills, m)
			}
		}
	}

	return AgentInfo{
		AgentID:         getStr(data, "agent_id"),
		Handle:          getStr(data, "handle"),
		Name:            name,
		Description:     desc,
		Metadata:        metadata,
		Capabilities:    capabilities,
		Skills:          skills,
		Price:           price,
		EndpointURL:     getStr(data, "endpoint_url"),
		OnChain:         onChain,
		IdentityAddress: getStr(data, "identity_address"),
	}
}

// PaginatedResponse is a generic paginated response from the API.
type PaginatedResponse struct {
	Items  []any `json:"items"`
	Total  int   `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// HasMore reports whether more items are available.
func (p PaginatedResponse) HasMore() bool { return p.Offset+p.Limit < p.Total }

// NextOffset returns the offset for the next page, or -1 if none.
func (p PaginatedResponse) NextOffset() int {
	if p.HasMore() {
		return p.Offset + p.Limit
	}
	return -1
}

// UserInfo describes a registered user.
type UserInfo struct {
	UserID        string `json:"user_id"`
	WalletAddress string `json:"wallet_address"`
	Email         string `json:"email,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// PriceInfo is price information for an agent or resource.
type PriceInfo struct {
	Identifier string         `json:"identifier"`
	Amount     float64        `json:"amount"`
	Currency   string         `json:"currency"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
