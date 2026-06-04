// Package wrappers exposes plain Go functions as A2A-compatible agent services,
// mirroring aip_sdk/wrappers/generic.py.
package wrappers

import (
	"os"
	"strconv"
	"strings"

	"github.com/unibaseio/aip-go-sdk/server"
	"github.com/unibaseio/aip-go-sdk/types"
)

// ExposeOptions configure ExposeAsA2A. Only Name is required.
type ExposeOptions struct {
	Name        string
	Description string
	Host        string
	Port        int
	Skills      []types.AgentSkillCard
	Streaming   bool
	RawResponse bool
	Version     string

	// Account / registration integration.
	UserID      string
	PrivyToken  string
	AIPEndpoint string
	GatewayURL  string
	Handle      string
	// AutoRegister defaults to true; set DisableAutoRegister to opt out.
	DisableAutoRegister bool
	ViaGateway          bool

	CostModel    *types.CostModel
	Currency     string
	EndpointURL  string
	Metadata     map[string]any
	ChainID      int
	JobOfferings []types.AgentJobOffering
	JobResources []types.AgentJobResource
}

// ExposeAsA2A exposes a text handler as an A2A agent service. Pass either
// handler (non-streaming) or streamHandler (streaming); the other may be nil.
func ExposeAsA2A(opts ExposeOptions, handler server.TextHandler, streamHandler server.StreamTextHandler) *server.Server {
	if opts.Description == "" {
		opts.Description = opts.Name + " agent"
	}
	if opts.Host == "" {
		opts.Host = "0.0.0.0"
	}
	if opts.Port == 0 {
		opts.Port = 8000
	}
	if opts.Version == "" {
		opts.Version = "1.0.0"
	}
	if opts.Currency == "" {
		opts.Currency = "USD"
	}
	if opts.ChainID == 0 {
		opts.ChainID = 97
	}

	if opts.Skills == nil {
		skillID := strings.ReplaceAll(strings.ToLower(opts.Name), " ", "_")
		opts.Skills = []types.AgentSkillCard{{ID: skillID, Name: opts.Name, Description: opts.Description}}
	}
	if opts.Handle == "" {
		opts.Handle = strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(opts.Name), " ", "."), "_", ".")
	}

	discoveryURL := opts.EndpointURL
	if discoveryURL == "" {
		discoveryURL = "http://" + opts.Host + ":" + strconv.Itoa(opts.Port)
	}

	skillNames := make([]string, 0, len(opts.Skills))
	for _, s := range opts.Skills {
		skillNames = append(skillNames, s.Name)
	}

	card := types.AgentCard{
		Type:               types.AgentCardType,
		Name:               opts.Name,
		Description:        opts.Description,
		URL:                discoveryURL,
		X402Support:        true,
		Active:             true,
		Version:            opts.Version,
		Skills:             opts.Skills,
		Capabilities:       types.AgentCapabilities{Streaming: opts.Streaming},
		Authentication:     types.NewAgentAuthentication(),
		SupportedTrust:     types.DefaultSupportedTrust(),
		TrustModels:        types.DefaultTrustModels(),
		Metadata:           opts.Metadata,
		JobOfferings:       opts.JobOfferings,
		JobResources:       opts.JobResources,
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Services: []types.AgentService{
			{Name: "A2A", Endpoint: discoveryURL + "/.well-known/agent-card.json", A2ASkills: skillNames},
			{Name: "web", Endpoint: discoveryURL},
		},
	}

	var taskHandler server.TaskHandler
	if opts.Streaming && streamHandler != nil {
		taskHandler = server.CreateStreamHandler(streamHandler, opts.RawResponse)
	} else {
		taskHandler = server.CreateSimpleHandler(handler, opts.RawResponse)
	}

	serverOpts := []server.Option{}
	resolvedUserID := opts.UserID
	if resolvedUserID == "" {
		resolvedUserID = os.Getenv("AIP_USER_ID")
	}
	autoRegister := !opts.DisableAutoRegister
	if resolvedUserID != "" && (autoRegister || opts.GatewayURL != "") {
		costModel := types.CostModel{}
		if opts.CostModel != nil {
			costModel = *opts.CostModel
		} else {
			base := 0.001
			costModel.BaseCallFee = &base
		}
		metadata := map[string]any{}
		for k, v := range opts.Metadata {
			metadata[k] = v
		}
		if opts.ViaGateway {
			metadata["via_gateway"] = true
		}
		privyToken := opts.PrivyToken
		if privyToken == "" {
			privyToken = os.Getenv("PRIVY_TOKEN")
		}
		regConfig := &server.RegistrationConfig{
			Handle:       opts.Handle,
			Name:         opts.Name,
			Description:  opts.Description,
			UserID:       resolvedUserID,
			PrivyToken:   privyToken,
			AIPEndpoint:  opts.AIPEndpoint,
			EndpointURL:  opts.EndpointURL,
			GatewayURL:   opts.GatewayURL,
			ViaGateway:   opts.ViaGateway,
			ChainID:      opts.ChainID,
			Currency:     opts.Currency,
			Skills:       skillConfigs(opts.Skills),
			CostModel:    &costModel,
			Metadata:     metadata,
			JobOfferings: opts.JobOfferings,
			JobResources: opts.JobResources,
		}
		serverOpts = append(serverOpts, server.WithRegistration(regConfig, autoRegister))
	}

	return server.New(card, taskHandler, opts.Host, opts.Port, serverOpts...)
}

func skillConfigs(cards []types.AgentSkillCard) []types.SkillConfig {
	out := make([]types.SkillConfig, 0, len(cards))
	for _, s := range cards {
		out = append(out, types.SkillConfig{Name: s.Name, Description: s.Description})
	}
	return out
}
