// Package types defines the SDK data models, mirroring aip_sdk/types.py.
// JSON field names follow the reference SDK (ERC-8004 agent cards use the
// camelCase keys defined by the spec).
package types

// AgentService is a service endpoint defined in an Agent Card.
type AgentService struct {
	Name      string   `json:"name"`
	Endpoint  string   `json:"endpoint"`
	Version   string   `json:"version,omitempty"`
	A2ASkills []string `json:"a2aSkills,omitempty"`
}

// AgentRegistration is an on-chain registration reference for an Agent Card.
type AgentRegistration struct {
	AgentID       string `json:"agentId"`
	AgentRegistry string `json:"agentRegistry"`
}

// AgentProvider identifies the entity providing/hosting the agent.
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities describes capabilities supported by the agent.
type AgentCapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

// AgentAuthentication describes authentication required to access the agent.
type AgentAuthentication struct {
	Schemes     []string `json:"schemes"`
	Credentials string   `json:"credentials,omitempty"`
}

// AgentSkillCard is a skill definition as represented in an Agent Card.
type AgentSkillCard struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// AgentJobOffering is a structured job offering as defined in an Agent Card
// (Virtuals ACP compatible).
type AgentJobOffering struct {
	ID            any            `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Type          string         `json:"type,omitempty"`
	Price         float64        `json:"price"`
	PriceV2       map[string]any `json:"priceV2,omitempty"`
	JobInput      string         `json:"jobInput,omitempty"`
	JobOutput     string         `json:"jobOutput,omitempty"`
	Requirement   map[string]any `json:"requirement,omitempty"`
	Deliverable   map[string]any `json:"deliverable,omitempty"`
	SLAMinutes    int            `json:"slaMinutes,omitempty"`
	RequiredFunds bool           `json:"requiredFunds,omitempty"`
	IsManagedFund bool           `json:"isManagedFund,omitempty"`
	Restricted    bool           `json:"restricted,omitempty"`
	Hide          bool           `json:"hide,omitempty"`
	Active        bool           `json:"active"`
}

// AgentJobResource is an auxiliary read-only resource defined in an Agent Card.
type AgentJobResource struct {
	ID          any    `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// AgentCard is a standard Agent Card following the ERC-8004 specification.
type AgentCard struct {
	Type               string              `json:"type"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	URL                string              `json:"url"`
	Image              string              `json:"image,omitempty"`
	IconURL            string              `json:"iconUrl,omitempty"`
	X402Support        bool                `json:"x402support"`
	Active             bool                `json:"active"`
	Version            string              `json:"version"`
	Services           []AgentService      `json:"services"`
	Registrations      []AgentRegistration `json:"registrations"`
	SupportedTrust     []string            `json:"supportedTrust"`
	Metadata           map[string]any      `json:"metadata,omitempty"`
	UserInterface      string              `json:"userInterface,omitempty"`
	FeedbackDataURI    string              `json:"FeedbackDataURI,omitempty"`
	Provider           *AgentProvider      `json:"provider,omitempty"`
	DocumentationURL   string              `json:"documentationUrl,omitempty"`
	Capabilities       AgentCapabilities   `json:"capabilities"`
	Authentication     AgentAuthentication `json:"authentication"`
	DefaultInputModes  []string            `json:"defaultInputModes"`
	DefaultOutputModes []string            `json:"defaultOutputModes"`
	Skills             []AgentSkillCard    `json:"skills"`
	JobOfferings       []AgentJobOffering  `json:"jobOfferings"`
	JobResources       []AgentJobResource  `json:"jobResources"`
	TrustModels        []string            `json:"trustModels"`
}

// AgentCardType is the ERC-8004 standard type identifier for agent cards.
const AgentCardType = "https://eips.ethereum.org/EIPS/eip-8004#registration-v1"

// NewAgentCapabilities returns capabilities with all flags false.
func NewAgentCapabilities() AgentCapabilities { return AgentCapabilities{} }

// NewAgentAuthentication returns authentication defaulting to Bearer.
func NewAgentAuthentication() AgentAuthentication {
	return AgentAuthentication{Schemes: []string{"Bearer"}}
}

// DefaultSupportedTrust is the default trust list for a new agent card.
func DefaultSupportedTrust() []string {
	return []string{"reputation", "crypto-economic", "tee-attestation"}
}

// DefaultTrustModels is the default trust-models list for a new agent card.
func DefaultTrustModels() []string {
	return []string{"feedback", "inference-validation", "tee-attestation"}
}
