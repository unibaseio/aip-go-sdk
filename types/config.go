package types

import (
	"fmt"
	"strings"
	"time"
)

// RunStatus is the status of a run execution in the AIP platform.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// TaskSpec specifies a task to be executed.
type TaskSpec struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Payload     map[string]any `json:"payload,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	ParentID    string         `json:"parent_id,omitempty"`
	CreatedAt   float64        `json:"created_at"`
}

// NewTaskSpec creates a TaskSpec with sensible defaults for ID and CreatedAt.
func NewTaskSpec() *TaskSpec {
	now := time.Now()
	return &TaskSpec{
		ID:        fmt.Sprintf("task_%d", now.Unix()),
		Payload:   map[string]any{},
		Metadata:  map[string]any{},
		CreatedAt: float64(now.Unix()),
	}
}

// Task is a task specification for SDK use.
type Task struct {
	TaskID        string         `json:"task_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	AssignedAgent string         `json:"assigned_agent,omitempty"`
}

// SkillInput defines a skill input parameter.
type SkillInput struct {
	Name        string `json:"name"`
	FieldType   string `json:"field_type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// SkillOutput defines a skill output parameter.
type SkillOutput struct {
	Name        string `json:"name"`
	FieldType   string `json:"field_type"`
	Description string `json:"description"`
}

// SkillConfig configures an agent skill.
type SkillConfig struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Inputs      []SkillInput  `json:"inputs,omitempty"`
	Outputs     []SkillOutput `json:"outputs,omitempty"`
}

// ToMap converts a SkillConfig into the API call dictionary form.
func (s SkillConfig) ToMap() map[string]any {
	inputs := make([]map[string]any, 0, len(s.Inputs))
	for _, i := range s.Inputs {
		inputs = append(inputs, map[string]any{
			"name":        i.Name,
			"field_type":  i.FieldType,
			"description": i.Description,
		})
	}
	outputs := make([]map[string]any, 0, len(s.Outputs))
	for _, o := range s.Outputs {
		outputs = append(outputs, map[string]any{
			"name":        o.Name,
			"field_type":  o.FieldType,
			"description": o.Description,
		})
	}
	return map[string]any{
		"name":        s.Name,
		"description": s.Description,
		"inputs":      inputs,
		"outputs":     outputs,
	}
}

// CostModel configures an agent's pricing.
type CostModel struct {
	BaseCallFee     *float64           `json:"base_call_fee,omitempty"`
	PerAgentCallFee *float64           `json:"per_agent_call_fee,omitempty"`
	PerUseFee       *float64           `json:"per_use_fee,omitempty"`
	PerWriteFee     *float64           `json:"per_write_fee,omitempty"`
	PerTokenFee     *float64           `json:"per_token_fee,omitempty"`
	CustomFees      map[string]float64 `json:"custom_fees"`
}

// ToMap renders the cost model, omitting nil fees, as the API expects.
func (c CostModel) ToMap() map[string]any {
	out := map[string]any{}
	if c.BaseCallFee != nil {
		out["base_call_fee"] = *c.BaseCallFee
	}
	if c.PerAgentCallFee != nil {
		out["per_agent_call_fee"] = *c.PerAgentCallFee
	}
	if c.PerUseFee != nil {
		out["per_use_fee"] = *c.PerUseFee
	}
	if c.PerWriteFee != nil {
		out["per_write_fee"] = *c.PerWriteFee
	}
	if c.PerTokenFee != nil {
		out["per_token_fee"] = *c.PerTokenFee
	}
	fees := c.CustomFees
	if fees == nil {
		fees = map[string]float64{}
	}
	out["custom_fees"] = fees
	return out
}

// AgentConfig configures an agent.
type AgentConfig struct {
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Handle       string             `json:"handle,omitempty"`
	Skills       []SkillConfig      `json:"skills,omitempty"`
	Capabilities []string           `json:"capabilities,omitempty"`
	CostModel    CostModel          `json:"cost_model"`
	Currency     string             `json:"currency"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
	EndpointURL  string             `json:"endpoint_url,omitempty"`
	JobOfferings []AgentJobOffering `json:"job_offerings,omitempty"`
	JobResources []AgentJobResource `json:"job_resources,omitempty"`
	ChainID      int                `json:"chain_id"`
}

// Price returns the primary price (base_call_fee), defaulting to 0.001.
func (c AgentConfig) Price() float64 {
	if c.CostModel.BaseCallFee != nil && *c.CostModel.BaseCallFee != 0 {
		return *c.CostModel.BaseCallFee
	}
	return 0.001
}

func (c AgentConfig) handleOrName() string {
	if c.Handle != "" {
		return c.Handle
	}
	return strings.ReplaceAll(strings.ToLower(c.Name), " ", "_")
}

// ToAgentCard synthesizes an ERC-8004 AgentCard from the config.
func (c AgentConfig) ToAgentCard(agentID, registryAddress string) AgentCard {
	if agentID == "" {
		agentID = "0"
	}
	handle := c.handleOrName()

	url := c.EndpointURL
	if url == "" {
		url = fmt.Sprintf("http://localhost:8000/agents/%s/", handle)
	}
	// The A2A service endpoint is the service BASE URL: consumers (e.g. the
	// platform's card refresher) append /.well-known/agent-card.json
	// themselves — including the path here doubled it up.
	a2aEndpoint := strings.TrimRight(url, "/")

	skillCards := make([]AgentSkillCard, 0, len(c.Skills))
	skillNames := make([]string, 0, len(c.Skills))
	for _, s := range c.Skills {
		skillCards = append(skillCards, AgentSkillCard{
			ID:          fmt.Sprintf("%s_%s", handle, s.Name),
			Name:        s.Name,
			Description: s.Description,
			Tags:        c.Capabilities,
			Examples:    []string{},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"application/json"},
		})
		skillNames = append(skillNames, s.Name)
	}

	var registrations []AgentRegistration
	if registryAddress != "" {
		registrations = []AgentRegistration{{AgentID: agentID, AgentRegistry: registryAddress}}
	}

	return AgentCard{
		Type:        AgentCardType,
		Name:        c.Name,
		Description: c.Description,
		URL:         url,
		X402Support: true,
		Active:      true,
		Version:     "1.0.0",
		Services: []AgentService{
			{Name: "A2A", Endpoint: a2aEndpoint, A2ASkills: skillNames},
			{Name: "web", Endpoint: url},
		},
		Registrations:      registrations,
		SupportedTrust:     DefaultSupportedTrust(),
		Metadata:           c.Metadata,
		Capabilities:       NewAgentCapabilities(),
		Authentication:     NewAgentAuthentication(),
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"application/json"},
		Skills:             skillCards,
		JobOfferings:       c.JobOfferings,
		JobResources:       c.JobResources,
		TrustModels:        DefaultTrustModels(),
		Provider:           &AgentProvider{Organization: "BitAgent", URL: "https://bitagent.io"},
	}
}

// ToRegistrationMap converts the config to the registration API format.
func (c AgentConfig) ToRegistrationMap() map[string]any {
	handle := c.handleOrName()

	skills := make([]map[string]any, 0, len(c.Skills))
	tasks := make([]map[string]any, 0, len(c.Skills))
	for _, s := range c.Skills {
		skills = append(skills, s.ToMap())
		tasks = append(tasks, map[string]any{"name": s.Name, "description": s.Description})
	}
	jobOfferings := c.JobOfferings
	if jobOfferings == nil {
		jobOfferings = []AgentJobOffering{}
	}
	jobResources := c.JobResources
	if jobResources == nil {
		jobResources = []AgentJobResource{}
	}

	return map[string]any{
		"handle":       handle,
		"card":         c.ToAgentCard("", ""),
		"skills":       skills,
		"tasks":        tasks,
		"cost_model":   c.CostModel.ToMap(),
		"price":        map[string]any{"amount": c.Price(), "currency": c.Currency},
		"jobOfferings": jobOfferings,
		"jobResources": jobResources,
		"metadata":     c.Metadata,
		"endpoint_url": c.EndpointURL,
		"chain_id":     c.ChainID,
	}
}

// AgentGroupConfig configures an agent group with intelligent routing.
type AgentGroupConfig struct {
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	MemberAgentIDs []string       `json:"member_agent_ids"`
	Price          float64        `json:"price"`
	Currency       string         `json:"currency"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ToRegistrationMap converts to the AIP group registration format.
func (g AgentGroupConfig) ToRegistrationMap() map[string]any {
	handle := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(g.Name), " ", "_"), "-", "_")
	return map[string]any{
		"group_name":       handle,
		"display_name":     g.Name,
		"member_agent_ids": g.MemberAgentIDs,
		"price":            map[string]any{"amount": g.Price, "currency": g.Currency},
		"metadata":         g.Metadata,
	}
}
