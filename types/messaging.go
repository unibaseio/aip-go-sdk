package types

import "encoding/json"

// MessageContext is context provided by the AIP platform for agent communication.
type MessageContext struct {
	RunID             string         `json:"run_id"`
	CallerID          string         `json:"caller_id"`
	ConversationID    string         `json:"conversation_id,omitempty"`
	PaymentAuthorized bool           `json:"payment_authorized"`
	CallerChain       []string       `json:"caller_chain,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// RoutingHints carries optional hints from the routing layer.
type RoutingHints struct {
	DetectedCategory  string         `json:"detected_category,omitempty"`
	ExtractedEntities map[string]any `json:"extracted_entities,omitempty"`
	Confidence        *float64       `json:"confidence,omitempty"`
	SuggestedTask     string         `json:"suggested_task,omitempty"`
}

// AgentMessage is the universal message format for agent communication.
type AgentMessage struct {
	Intent         string         `json:"intent"`
	Context        MessageContext `json:"context"`
	Hints          *RoutingHints  `json:"hints,omitempty"`
	StructuredData map[string]any `json:"structured_data,omitempty"`
}

// IntentAsJSON tries to parse the intent string as JSON, returning nil on failure.
func (m AgentMessage) IntentAsJSON() map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(m.Intent), &out); err != nil {
		return nil
	}
	return out
}

// NewAgentMessage is a convenience factory for an AgentMessage.
func NewAgentMessage(intent, runID, callerID, conversationID string, hints *RoutingHints, metadata map[string]any) AgentMessage {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return AgentMessage{
		Intent: intent,
		Context: MessageContext{
			RunID:             runID,
			CallerID:          callerID,
			ConversationID:    conversationID,
			PaymentAuthorized: true,
			Metadata:          metadata,
		},
		Hints: hints,
	}
}

// AgentMessageFromMap builds an AgentMessage from a decoded "intent"/"context"
// map (the canonical AgentMessage JSON shape).
func AgentMessageFromMap(data map[string]any) AgentMessage {
	intent, _ := data["intent"].(string)
	ctxData, _ := data["context"].(map[string]any)
	am := AgentMessage{Intent: intent, Context: MessageContextFromMap(ctxData)}
	if hintsData, ok := data["hints"].(map[string]any); ok && len(hintsData) > 0 {
		h := RoutingHintsFromMap(hintsData)
		am.Hints = &h
	}
	if sd, ok := data["structured_data"].(map[string]any); ok {
		am.StructuredData = sd
	}
	return am
}

// MessageContextFromMap decodes a MessageContext from a generic map.
func MessageContextFromMap(data map[string]any) MessageContext {
	if data == nil {
		return MessageContext{PaymentAuthorized: true}
	}
	b, _ := json.Marshal(data)
	ctx := MessageContext{PaymentAuthorized: true}
	_ = json.Unmarshal(b, &ctx)
	return ctx
}

// RoutingHintsFromMap decodes RoutingHints from a generic map.
func RoutingHintsFromMap(data map[string]any) RoutingHints {
	var h RoutingHints
	b, _ := json.Marshal(data)
	_ = json.Unmarshal(b, &h)
	return h
}

// AgentResponse is the standard response format from agents.
type AgentResponse struct {
	Content  string         `json:"content"`
	Data     map[string]any `json:"data,omitempty"`
	Success  bool           `json:"success"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SuccessResponse builds a successful AgentResponse.
func SuccessResponse(content string, data map[string]any) AgentResponse {
	if data == nil {
		data = map[string]any{}
	}
	return AgentResponse{Content: content, Data: data, Success: true}
}

// ErrorResponse builds a failed AgentResponse.
func ErrorResponse(err string) AgentResponse {
	return AgentResponse{Content: err, Success: false, Error: err}
}
