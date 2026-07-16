package a2a

import (
	"strings"

	"github.com/unibaseio/aip-go-sdk/core"
	"github.com/unibaseio/aip-go-sdk/types"
)

// GenerateCardOptions customize agent card generation. Zero values fall back to
// sensible defaults derived from the identity.
type GenerateCardOptions struct {
	Description  string
	Skills       []types.AgentSkillCard
	Capabilities *types.AgentCapabilities
	Provider     *types.AgentProvider
	Version      string
}

// GenerateAgentCard builds an ERC-8004 AgentCard from an AgentIdentity.
func GenerateAgentCard(identity core.AgentIdentity, baseURL string, opts GenerateCardOptions) types.AgentCard {
	baseURL = strings.TrimRight(baseURL, "/")

	description := opts.Description
	if description == "" {
		description = "AI agent: " + identity.Name
		if d, ok := identity.Metadata["description"].(string); ok && d != "" {
			description = d
		}
	}

	capabilities := types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: true}
	if opts.Capabilities != nil {
		capabilities = *opts.Capabilities
	}

	skills := opts.Skills
	if skills == nil {
		if raw, ok := identity.Metadata["skills"].([]any); ok && len(raw) > 0 {
			for _, item := range raw {
				sd, _ := item.(map[string]any)
				skills = append(skills, skillCardFromMap(sd, strings.ToLower(identity.Name)+"-skill", identity.Name, description))
			}
		} else {
			skills = []types.AgentSkillCard{{
				ID:          identity.AgentID + "-default",
				Name:        identity.Name,
				Description: description,
				Tags:        []string{string(identity.AgentType), "unibase"},
			}}
		}
	}

	version := opts.Version
	if version == "" {
		version = "1.0.0"
	}

	return buildCard(identity.Name, description, baseURL, version, capabilities, skills, opts.Provider)
}

// AgentCardFromMetadata builds an AgentCard from a raw metadata map.
func AgentCardFromMetadata(metadata map[string]any, baseURL string) types.AgentCard {
	baseURL = strings.TrimRight(baseURL, "/")
	name, _ := metadata["name"].(string)
	if name == "" {
		name = "agent"
	}
	description, _ := metadata["description"].(string)
	if description == "" {
		description = "AI agent: " + name
	}
	version, _ := metadata["version"].(string)
	if version == "" {
		version = "1.0.0"
	}

	var skills []types.AgentSkillCard
	if raw, ok := metadata["skills"].([]any); ok {
		for _, item := range raw {
			sd, _ := item.(map[string]any)
			skills = append(skills, skillCardFromMap(sd, name+"_skill", "skill", ""))
		}
	}

	caps := types.AgentCapabilities{Streaming: true, StateTransitionHistory: true}
	if cm, ok := metadata["capabilities"].(map[string]any); ok {
		caps = types.AgentCapabilities{
			Streaming:              boolOr(cm["streaming"], true),
			PushNotifications:      boolOr(cm["push_notifications"], false),
			StateTransitionHistory: boolOr(cm["state_transition_history"], true),
		}
	}

	var provider *types.AgentProvider
	if pm, ok := metadata["provider"].(map[string]any); ok {
		org, _ := pm["organization"].(string)
		if org == "" {
			org = "BitAgent"
		}
		url, _ := pm["url"].(string)
		provider = &types.AgentProvider{Organization: org, URL: url}
	}

	return buildCard(name, description, baseURL, version, caps, skills, provider)
}

func buildCard(name, description, baseURL, version string, caps types.AgentCapabilities, skills []types.AgentSkillCard, provider *types.AgentProvider) types.AgentCard {
	skillNames := make([]string, 0, len(skills))
	for _, s := range skills {
		skillNames = append(skillNames, s.Name)
	}
	return types.AgentCard{
		Type:           types.AgentCardType,
		Name:           name,
		Description:    description,
		URL:            baseURL,
		X402Support:    true,
		Active:         true,
		Version:        version,
		Capabilities:   caps,
		Authentication: types.NewAgentAuthentication(),
		Skills:         skills,
		Provider:       provider,
		SupportedTrust: types.DefaultSupportedTrust(),
		TrustModels:    types.DefaultTrustModels(),
		Services: []types.AgentService{
			// Base URL only — consumers append /.well-known/agent-card.json.
			{Name: "A2A", Endpoint: baseURL, A2ASkills: skillNames},
			{Name: "web", Endpoint: baseURL},
		},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
	}
}

func skillCardFromMap(sd map[string]any, defaultID, defaultName, defaultDesc string) types.AgentSkillCard {
	getStr := func(k, fallback string) string {
		if s, ok := sd[k].(string); ok && s != "" {
			return s
		}
		return fallback
	}
	getStrs := func(k string) []string {
		var out []string
		if raw, ok := sd[k].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return types.AgentSkillCard{
		ID:          getStr("id", defaultID),
		Name:        getStr("name", defaultName),
		Description: getStr("description", defaultDesc),
		Tags:        getStrs("tags"),
		Examples:    getStrs("examples"),
	}
}

func boolOr(v any, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}
