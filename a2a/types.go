package a2a

// InvokeRequest is the request format for invoking an agent.
type InvokeRequest struct {
	Message    string         `json:"message"`
	Context    map[string]any `json:"context,omitempty"`
	DomainHint string         `json:"domain_hint,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
}

// InvokeResponse is the response from an agent invocation.
type InvokeResponse struct {
	RunID    string           `json:"run_id"`
	AgentID  string           `json:"agent_id"`
	Success  bool             `json:"success"`
	Content  string           `json:"content"`
	Data     map[string]any   `json:"data,omitempty"`
	Error    string           `json:"error,omitempty"`
	Payments []map[string]any `json:"payments,omitempty"`
}

// StreamResponse wraps a streaming event emitted from an agent to a client.
// Exactly one of the event fields is typically set; RawContent carries an
// unstructured text chunk.
type StreamResponse struct {
	Task           *Task
	Message        *Message
	StatusUpdate   *TaskStatusUpdateEvent
	ArtifactUpdate *TaskArtifactUpdateEvent
	RawContent     string
}

// Event returns the underlying event object, preferring task, then message,
// then status update, then artifact update.
func (s *StreamResponse) Event() any {
	switch {
	case s.Task != nil:
		return s.Task
	case s.Message != nil:
		return s.Message
	case s.StatusUpdate != nil:
		return s.StatusUpdate
	case s.ArtifactUpdate != nil:
		return s.ArtifactUpdate
	default:
		return nil
	}
}

// Standard A2A protocol error codes (JSON-RPC and A2A-specific).
const (
	ErrParseError                    = -32700
	ErrInvalidRequest                = -32600
	ErrMethodNotFound                = -32601
	ErrInvalidParams                 = -32602
	ErrInternalError                 = -32603
	ErrTaskNotFound                  = -32001
	ErrTaskNotCancelable             = -32002
	ErrPushNotificationNotSupported  = -32003
	ErrUnsupportedOperation          = -32004
	ErrContentTypeNotSupported       = -32005
	ErrInvalidAgentResponse          = -32006
)
