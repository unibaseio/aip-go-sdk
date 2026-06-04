// Package messaging defines AIP-specific extensions embedded in A2A messages,
// mirroring aip_sdk/messaging/types.py and extensions.py.
package messaging

// AIPMetadataKey is the key under which AIP metadata is stored in an A2A
// message's metadata map.
const AIPMetadataKey = "_aip"

// PaymentEvent is a payment event that occurred during agent execution.
type PaymentEvent struct {
	Type          string         `json:"type"`
	AgentID       string         `json:"agent_id"`
	Amount        float64        `json:"amount"`
	Currency      string         `json:"currency"`
	Protocol      string         `json:"protocol"`
	Timestamp     string         `json:"timestamp"`
	Status        string         `json:"status"`
	TransactionID string         `json:"transaction_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// RoutingHints carries hints for routing messages to appropriate agents.
type RoutingHints struct {
	DetectedCategory  string         `json:"detected_category,omitempty"`
	SuggestedAgents   []string       `json:"suggested_agents,omitempty"`
	Confidence        *float64       `json:"confidence,omitempty"`
	ExtractedEntities map[string]any `json:"extracted_entities,omitempty"`
}

// AIPMetadata is the AIP platform metadata embedded in A2A messages, stored in
// message.metadata["_aip"].
type AIPMetadata struct {
	RunID             string         `json:"run_id"`
	CallerID          string         `json:"caller_id"`
	CallerChain       []string       `json:"caller_chain,omitempty"`
	ConversationID    string         `json:"conversation_id,omitempty"`
	PaymentAuthorized bool           `json:"payment_authorized"`
	PaymentEvents     []PaymentEvent `json:"payment_events,omitempty"`
	RoutingHints      *RoutingHints  `json:"routing_hints,omitempty"`
	TargetAgent       string         `json:"target_agent,omitempty"`
	MemoryScope       string         `json:"memory_scope,omitempty"`
	SessionID         string         `json:"session_id,omitempty"`
	Custom            map[string]any `json:"custom,omitempty"`
}

// SpawnChild creates child metadata for a nested agent call, preserving the
// caller chain. newRunID defaults to the parent run ID when empty.
func (m *AIPMetadata) SpawnChild(targetAgent, newRunID string) *AIPMetadata {
	newChain := append([]string{}, m.CallerChain...)
	if len(newChain) == 0 || newChain[len(newChain)-1] != m.CallerID {
		newChain = append(newChain, m.CallerID)
	}
	if newRunID == "" {
		newRunID = m.RunID
	}
	custom := map[string]any{}
	for k, v := range m.Custom {
		custom[k] = v
	}
	return &AIPMetadata{
		RunID:             newRunID,
		CallerID:          targetAgent,
		CallerChain:       newChain,
		ConversationID:    m.ConversationID,
		PaymentAuthorized: m.PaymentAuthorized,
		PaymentEvents:     []PaymentEvent{},
		TargetAgent:       targetAgent,
		MemoryScope:       m.MemoryScope,
		SessionID:         m.SessionID,
		Custom:            custom,
	}
}

// ToMap serializes the metadata into a map suitable for embedding in A2A
// message metadata.
func (m *AIPMetadata) ToMap() map[string]any {
	out := map[string]any{
		"run_id":             m.RunID,
		"caller_id":          m.CallerID,
		"payment_authorized": m.PaymentAuthorized,
	}
	if len(m.CallerChain) > 0 {
		out["caller_chain"] = m.CallerChain
	}
	if m.ConversationID != "" {
		out["conversation_id"] = m.ConversationID
	}
	if len(m.PaymentEvents) > 0 {
		out["payment_events"] = m.PaymentEvents
	}
	if m.RoutingHints != nil {
		out["routing_hints"] = m.RoutingHints
	}
	if m.TargetAgent != "" {
		out["target_agent"] = m.TargetAgent
	}
	if m.MemoryScope != "" {
		out["memory_scope"] = m.MemoryScope
	}
	if m.SessionID != "" {
		out["session_id"] = m.SessionID
	}
	if len(m.Custom) > 0 {
		out["custom"] = m.Custom
	}
	return out
}
