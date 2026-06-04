// Package agent provides the AIP context envelope embedded in A2A messages and
// the agent-side helpers, mirroring aip_sdk/agent/context.py.
package agent

import (
	"github.com/google/uuid"

	"github.com/unibaseio/aip-go-sdk/a2a"
)

// AIPContextKey is the metadata key under which AIP context is embedded in A2A
// messages.
const AIPContextKey = "aip_context"

// PaymentContextData is serializable payment context for embedding in A2A messages.
type PaymentContextData struct {
	RunID  string   `json:"run_id"`
	Caller string   `json:"caller"`
	Actor  string   `json:"actor"`
	Chain  []string `json:"chain,omitempty"`
}

// AIPContext is the AIP system context embedded in A2A message metadata.
type AIPContext struct {
	RunID          string              `json:"run_id"`
	CallerAgent    string              `json:"caller_agent"`
	CallerChain    []string            `json:"caller_chain,omitempty"`
	PaymentContext *PaymentContextData `json:"payment_context,omitempty"`
	MemoryScope    string              `json:"memory_scope,omitempty"`
	EventBusID     string              `json:"event_bus_id,omitempty"`
	Metadata       map[string]any      `json:"metadata,omitempty"`
}

// ToMap serializes the context into a map for embedding in A2A metadata.
func (c *AIPContext) ToMap() map[string]any {
	out := map[string]any{
		"run_id":       c.RunID,
		"caller_agent": c.CallerAgent,
		"caller_chain": c.CallerChain,
	}
	if c.PaymentContext != nil {
		out["payment_context"] = map[string]any{
			"run_id": c.PaymentContext.RunID,
			"caller": c.PaymentContext.Caller,
			"actor":  c.PaymentContext.Actor,
			"chain":  c.PaymentContext.Chain,
		}
	}
	if c.MemoryScope != "" {
		out["memory_scope"] = c.MemoryScope
	}
	if c.EventBusID != "" {
		out["event_bus_id"] = c.EventBusID
	}
	if len(c.Metadata) > 0 {
		out["metadata"] = c.Metadata
	}
	return out
}

// SpawnChild creates a child context for delegating to another agent.
func (c *AIPContext) SpawnChild(targetAgent string) *AIPContext {
	newChain := append(append([]string{}, c.CallerChain...), c.CallerAgent)

	var childPayment *PaymentContextData
	if c.PaymentContext != nil {
		childPayment = &PaymentContextData{
			RunID:  c.PaymentContext.RunID,
			Caller: c.CallerAgent,
			Actor:  targetAgent,
			Chain:  newChain,
		}
	}
	meta := map[string]any{}
	for k, v := range c.Metadata {
		meta[k] = v
	}
	return &AIPContext{
		RunID:          c.RunID,
		CallerAgent:    c.CallerAgent,
		CallerChain:    newChain,
		PaymentContext: childPayment,
		MemoryScope:    c.MemoryScope,
		EventBusID:     c.EventBusID,
		Metadata:       meta,
	}
}

// AIPContextFromMap deserializes an AIPContext from a metadata map.
func AIPContextFromMap(data map[string]any) *AIPContext {
	if data == nil {
		return nil
	}
	getStr := func(k string) string {
		s, _ := data[k].(string)
		return s
	}
	ctx := &AIPContext{
		RunID:       getStr("run_id"),
		CallerAgent: getStr("caller_agent"),
		MemoryScope: getStr("memory_scope"),
		EventBusID:  getStr("event_bus_id"),
	}
	if chain, ok := data["caller_chain"].([]any); ok {
		for _, c := range chain {
			if s, ok := c.(string); ok {
				ctx.CallerChain = append(ctx.CallerChain, s)
			}
		}
	}
	if pc, ok := data["payment_context"].(map[string]any); ok {
		p := &PaymentContextData{}
		p.RunID, _ = pc["run_id"].(string)
		p.Caller, _ = pc["caller"].(string)
		p.Actor, _ = pc["actor"].(string)
		if chain, ok := pc["chain"].([]any); ok {
			for _, c := range chain {
				if s, ok := c.(string); ok {
					p.Chain = append(p.Chain, s)
				}
			}
		}
		ctx.PaymentContext = p
	}
	if md, ok := data["metadata"].(map[string]any); ok {
		ctx.Metadata = md
	}
	return ctx
}

// WrapMessage embeds AIP context into an A2A message's metadata.
func WrapMessage(message *a2a.Message, ctx *AIPContext) *a2a.Message {
	newMeta := map[string]any{}
	for k, v := range message.Metadata {
		newMeta[k] = v
	}
	newMeta[AIPContextKey] = ctx.ToMap()
	id := message.ID
	if id == "" {
		id = uuid.NewString()
	}
	return &a2a.Message{
		ID:       id,
		Role:     message.Role,
		Parts:    message.Parts,
		Metadata: newMeta,
	}
}

// UnwrapMessage extracts AIP context from a message, returning a copy with the
// context removed and the extracted context (nil if absent).
func UnwrapMessage(message *a2a.Message) (*a2a.Message, *AIPContext) {
	if message.Metadata == nil {
		return message, nil
	}
	meta := map[string]any{}
	for k, v := range message.Metadata {
		if k == AIPContextKey {
			continue
		}
		meta[k] = v
	}
	var ctx *AIPContext
	if raw, ok := message.Metadata[AIPContextKey].(map[string]any); ok {
		ctx = AIPContextFromMap(raw)
	}
	id := message.ID
	if id == "" {
		id = uuid.NewString()
	}
	var metaOut map[string]any
	if len(meta) > 0 {
		metaOut = meta
	}
	return &a2a.Message{
		ID:       id,
		Role:     message.Role,
		Parts:    message.Parts,
		Metadata: metaOut,
	}, ctx
}

// ExtractAIPContext extracts AIP context from a message without modifying it.
func ExtractAIPContext(message *a2a.Message) *AIPContext {
	if message.Metadata == nil {
		return nil
	}
	if raw, ok := message.Metadata[AIPContextKey].(map[string]any); ok {
		return AIPContextFromMap(raw)
	}
	return nil
}
