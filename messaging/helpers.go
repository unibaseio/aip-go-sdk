package messaging

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/unibase-aip-sdk-go/a2a"
	"github.com/unibaseio/unibase-aip-sdk-go/types"
)

// AgentMessageFromA2A parses an A2A Message into a types.AgentMessage.
func AgentMessageFromA2A(msg *a2a.Message) types.AgentMessage {
	text := a2a.GetMessageText(msg)

	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return types.AgentMessage{Intent: text, Context: types.MessageContext{PaymentAuthorized: true}}
	}

	_, hasIntent := data["intent"]
	_, hasCtx := data["context"]
	if hasIntent && hasCtx {
		return types.AgentMessageFromMap(data)
	}

	if taskData, ok := data["task"].(map[string]any); ok {
		payload, _ := taskData["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
		}
		intent, _ := payload["intent"].(string)
		if intent == "" {
			intent, _ = taskData["description"].(string)
		}
		ctxData, _ := payload["context"].(map[string]any)
		if ctxData == nil {
			ctxData = map[string]any{"run_id": taskData["task_id"], "caller_id": "unknown"}
		}
		am := types.AgentMessage{Intent: intent, Context: types.MessageContextFromMap(ctxData)}
		if hintsData, ok := payload["hints"].(map[string]any); ok && len(hintsData) > 0 {
			h := types.RoutingHintsFromMap(hintsData)
			am.Hints = &h
		}
		if sd, ok := payload["structured_data"].(map[string]any); ok {
			am.StructuredData = sd
		}
		return am
	}

	return types.AgentMessage{Intent: text, Context: types.MessageContext{PaymentAuthorized: true}, StructuredData: data}
}

// CreateUserMessage builds a user A2A message carrying AIP metadata. This is
// the standard format for internal AIP messages.
func CreateUserMessage(text string, meta *AIPMetadata, structuredData map[string]any, messageID string) *a2a.Message {
	if messageID == "" {
		messageID = uuid.NewString()
	}
	if meta == nil {
		meta = &AIPMetadata{PaymentAuthorized: true}
	}
	if structuredData != nil {
		meta.Custom = structuredData
	}
	parts := a2a.ContentParts{a2a.NewTextPart(text)}
	if structuredData != nil {
		parts = append(parts, a2a.NewDataPart(structuredData))
	}
	return &a2a.Message{
		ID:       messageID,
		Role:     a2a.RoleUser,
		Parts:    parts,
		Metadata: map[string]any{AIPMetadataKey: meta.ToMap()},
	}
}

// CreateAgentMessage builds an agent response A2A message.
func CreateAgentMessage(text string, success bool, output map[string]any, errMsg string, meta *AIPMetadata, messageID string) *a2a.Message {
	if messageID == "" {
		messageID = uuid.NewString()
	}
	metadata := map[string]any{"success": success}
	if errMsg != "" {
		metadata["error"] = errMsg
	}
	if output != nil {
		metadata["output"] = output
	}
	if meta != nil {
		metadata[AIPMetadataKey] = meta.ToMap()
	}
	parts := a2a.ContentParts{a2a.NewTextPart(text)}
	if output != nil {
		parts = append(parts, a2a.NewDataPart(output))
	}
	return &a2a.Message{
		ID:       messageID,
		Role:     a2a.RoleAgent,
		Parts:    parts,
		Metadata: metadata,
	}
}

// GetAIPMetadata extracts AIP metadata from an A2A message, returning nil if absent.
func GetAIPMetadata(message *a2a.Message) *AIPMetadata {
	if message == nil || message.Metadata == nil {
		return nil
	}
	raw, ok := message.Metadata[AIPMetadataKey]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var meta AIPMetadata
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil
	}
	return &meta
}

// SetAIPMetadata returns a new message with the given AIP metadata set.
func SetAIPMetadata(message *a2a.Message, meta *AIPMetadata) *a2a.Message {
	newMeta := map[string]any{}
	for k, v := range message.Metadata {
		newMeta[k] = v
	}
	newMeta[AIPMetadataKey] = meta.ToMap()
	return &a2a.Message{
		ID:       message.ID,
		Role:     message.Role,
		Parts:    message.Parts,
		Metadata: newMeta,
	}
}

// GetText extracts the text content from a message.
func GetText(message *a2a.Message) string { return a2a.GetMessageText(message) }

// GetStructuredData extracts structured data from a message, checking data
// parts, then metadata.output, then AIP custom metadata.
func GetStructuredData(message *a2a.Message) map[string]any {
	if message == nil {
		return nil
	}
	for _, p := range message.Parts {
		if data, ok := a2a.PartData(p); ok && data != nil {
			return data
		}
	}
	if message.Metadata != nil {
		if out, ok := message.Metadata["output"].(map[string]any); ok {
			return out
		}
	}
	if meta := GetAIPMetadata(message); meta != nil && len(meta.Custom) > 0 {
		return meta.Custom
	}
	return nil
}

// AddPaymentEvent returns a new message with a payment event appended to its
// AIP metadata.
func AddPaymentEvent(message *a2a.Message, agentID string, amount float64, currency, status, transactionID string, extra map[string]any) *a2a.Message {
	meta := GetAIPMetadata(message)
	if meta == nil {
		meta = &AIPMetadata{PaymentAuthorized: true}
	}
	if currency == "" {
		currency = "USD"
	}
	if status == "" {
		status = "settled"
	}
	meta.PaymentEvents = append(meta.PaymentEvents, PaymentEvent{
		Type:          "payment",
		AgentID:       agentID,
		Amount:        amount,
		Currency:      currency,
		Protocol:      "X402",
		Timestamp:     time.Now().Format(time.RFC3339),
		Status:        status,
		TransactionID: transactionID,
		Metadata:      extra,
	})
	return SetAIPMetadata(message, meta)
}

// GetPaymentEvents returns all payment events from a message.
func GetPaymentEvents(message *a2a.Message) []PaymentEvent {
	if meta := GetAIPMetadata(message); meta != nil {
		return meta.PaymentEvents
	}
	return nil
}

// GetTotalCost sums settled payment events in the given currency.
func GetTotalCost(message *a2a.Message, currency string) float64 {
	if currency == "" {
		currency = "USD"
	}
	var total float64
	for _, e := range GetPaymentEvents(message) {
		if e.Currency == currency && e.Status == "settled" {
			total += e.Amount
		}
	}
	return total
}

// GetTargetAgent returns the target agent from a message's AIP metadata.
func GetTargetAgent(message *a2a.Message) string {
	if meta := GetAIPMetadata(message); meta != nil {
		return meta.TargetAgent
	}
	return ""
}

// SpawnChildMessage creates a child message for a nested agent call, preserving
// the caller chain.
func SpawnChildMessage(parent *a2a.Message, childText, targetAgent, newRunID string) *a2a.Message {
	parentMeta := GetAIPMetadata(parent)
	if parentMeta == nil {
		runID := newRunID
		if runID == "" {
			runID = uuid.NewString()
		}
		return CreateUserMessage(childText, &AIPMetadata{
			RunID:             runID,
			CallerID:          "unknown",
			PaymentAuthorized: true,
			TargetAgent:       targetAgent,
		}, nil, "")
	}
	childMeta := parentMeta.SpawnChild(targetAgent, newRunID)
	return &a2a.Message{
		ID:       uuid.NewString(),
		Role:     a2a.RoleUser,
		Parts:    a2a.ContentParts{a2a.NewTextPart(childText)},
		Metadata: map[string]any{AIPMetadataKey: childMeta.ToMap()},
	}
}
