// Package core defines foundational types shared across the Unibase AIP SDK,
// mirroring aip_sdk/core/types.py.
package core

// AgentType enumerates the supported agent implementation kinds.
type AgentType string

const (
	AgentTypeAIP       AgentType = "aip"
	AgentTypeClaude    AgentType = "claude"
	AgentTypeLangChain AgentType = "langchain"
	AgentTypeOpenAI    AgentType = "openai"
	AgentTypeCustom    AgentType = "custom"
)

// AgentIdentity holds identity information for an agent.
type AgentIdentity struct {
	AgentID       string         `json:"agent_id"`
	Name          string         `json:"name"`
	AgentType     AgentType      `json:"agent_type"`
	PublicKey     string         `json:"public_key,omitempty"`
	WalletAddress string         `json:"wallet_address,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// MemoryRecord represents a stored memory entry.
type MemoryRecord struct {
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id"`
	Content   map[string]any `json:"content"`
	Timestamp float64        `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// DAUploadResult describes the outcome of a data-availability upload.
type DAUploadResult struct {
	TransactionHash string  `json:"transaction_hash"`
	DAURL           string  `json:"da_url"`
	Size            int     `json:"size"`
	Timestamp       float64 `json:"timestamp"`
}
