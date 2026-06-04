// Package registry provides a thin agent-management wrapper around the platform
// client, mirroring aip_sdk/registry/registry.py. Membase initialization and
// framework-specific type adapters from the Python SDK are omitted.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/unibase-aip-sdk-go/a2a"
	"github.com/unibaseio/unibase-aip-sdk-go/aiperr"
	"github.com/unibaseio/unibase-aip-sdk-go/core"
	"github.com/unibaseio/unibase-aip-sdk-go/gateway"
	"github.com/unibaseio/unibase-aip-sdk-go/internal/log"
	"github.com/unibaseio/unibase-aip-sdk-go/platform"
	"github.com/unibaseio/unibase-aip-sdk-go/types"
)

var logger = log.Get("registry.registry")

// RegistrationMode selects how agents are registered.
type RegistrationMode string

const (
	// ModeDirect registers directly with the AIP platform.
	ModeDirect RegistrationMode = "direct"
	// ModeGateway registers through a gateway (for agents behind NAT/firewalls).
	ModeGateway RegistrationMode = "gateway"
)

// Client manages agents, wrapping the AIP platform client and (optionally) a gateway.
type Client struct {
	aipEndpoint     string
	mode            RegistrationMode
	gatewayURL      string
	agentBackendURL string

	aip       *platform.Client
	gateway   *gateway.Client
	a2aClient *a2a.Client

	mu              sync.Mutex
	identities      map[string]core.AgentIdentity
	agents          map[string]any
	discoveredAgents map[string]*types.AgentCard
}

// Config configures a registry Client.
type Config struct {
	AIPEndpoint     string
	Mode            RegistrationMode
	GatewayURL      string
	AgentBackendURL string
}

// New creates a registry Client. GATEWAY mode requires GatewayURL and
// AgentBackendURL (falling back to GATEWAY_URL / AGENT_BACKEND_URL env vars).
func New(cfg Config) (*Client, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeDirect
	}
	if cfg.AIPEndpoint == "" {
		cfg.AIPEndpoint = platform.DefaultBaseURL()
	}
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = os.Getenv("GATEWAY_URL")
	}
	if cfg.AgentBackendURL == "" {
		cfg.AgentBackendURL = os.Getenv("AGENT_BACKEND_URL")
	}
	if cfg.Mode == ModeGateway {
		if cfg.GatewayURL == "" {
			return nil, aiperr.Configuration("gateway_url is required when using GATEWAY mode")
		}
		if cfg.AgentBackendURL == "" {
			return nil, aiperr.Configuration("agent_backend_url is required when using GATEWAY mode")
		}
	}

	c := &Client{
		aipEndpoint:      cfg.AIPEndpoint,
		mode:             cfg.Mode,
		gatewayURL:       cfg.GatewayURL,
		agentBackendURL:  cfg.AgentBackendURL,
		aip:              platform.New(cfg.AIPEndpoint),
		a2aClient:        a2a.NewClient(0, nil),
		identities:       map[string]core.AgentIdentity{},
		agents:           map[string]any{},
		discoveredAgents: map[string]*types.AgentCard{},
	}
	if cfg.Mode == ModeGateway {
		c.gateway = gateway.NewClient(cfg.GatewayURL, 0)
	}
	logger.Infof("AgentRegistry initialized in %s mode", cfg.Mode)
	return c, nil
}

// HealthCheck reports the health of the platform (and gateway in gateway mode).
func (c *Client) HealthCheck(ctx context.Context) map[string]bool {
	results := map[string]bool{"aip_platform": c.aip.HealthCheck(ctx)}
	if c.mode == ModeGateway && c.gateway != nil {
		results["gateway"] = c.gateway.HealthCheck(ctx)
	}
	return results
}

// WaitForServices polls HealthCheck until all services are healthy or attempts run out.
func (c *Client) WaitForServices(ctx context.Context, maxAttempts int, interval time.Duration) bool {
	for i := 0; i < maxAttempts; i++ {
		allHealthy := true
		for _, ok := range c.HealthCheck(ctx) {
			if !ok {
				allHealthy = false
				break
			}
		}
		if allHealthy {
			return true
		}
		if i < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(interval):
			}
		}
	}
	return false
}

// RegisterAgentOptions configure agent registration.
type RegisterAgentOptions struct {
	WalletAddress string
	Metadata      map[string]any
	UserID        string
	Force         bool
	CostModel     *types.CostModel
	Currency      string
}

// RegisterAgent registers a new agent and returns its identity. Registration
// failures fall back to a locally generated agent ID.
func (c *Client) RegisterAgent(ctx context.Context, name string, agentType core.AgentType, opts RegisterAgentOptions) (core.AgentIdentity, error) {
	if opts.Metadata == nil {
		opts.Metadata = map[string]any{}
	}
	if opts.UserID == "" {
		opts.UserID = "system"
	}
	if opts.Currency == "" {
		opts.Currency = "USD"
	}
	costModel := types.CostModel{}
	if opts.CostModel != nil {
		costModel = *opts.CostModel
	} else {
		base := 0.001
		costModel.BaseCallFee = &base
	}

	handle, _ := opts.Metadata["handle"].(string)
	if handle == "" {
		handle = strings.ReplaceAll(strings.ToLower(name), " ", "_")
	}
	description, _ := opts.Metadata["description"].(string)
	var capabilities []string
	if raw, ok := opts.Metadata["capabilities"].([]string); ok {
		capabilities = raw
	}

	meta := map[string]any{
		"agent_type":     string(agentType),
		"wallet_address": opts.WalletAddress,
		"mode":           string(c.mode),
	}
	for k, v := range opts.Metadata {
		meta[k] = v
	}

	agentConfig := types.AgentConfig{
		Name:         name,
		Description:  description,
		Handle:       handle,
		Capabilities: capabilities,
		CostModel:    costModel,
		Currency:     opts.Currency,
		Metadata:     meta,
		ChainID:      97,
	}

	var endpointURL string
	if c.mode == ModeGateway {
		gwResult, err := c.gateway.RegisterAgent(ctx, name, c.agentBackendURL, map[string]any{"agent_type": string(agentType)}, opts.Force)
		if err != nil {
			logger.Errorf("Gateway registration failed: %v", err)
			return core.AgentIdentity{}, aiperr.Registry("Failed to register agent with gateway: " + err.Error())
		}
		endpointURL, _ = gwResult["gateway_url"].(string)
		meta["endpoint_url"] = endpointURL
		meta["gateway_mode"] = true
		agentConfig.Metadata = meta
	}

	var agentID string
	if result, err := c.aip.RegisterAgent(ctx, agentConfig, opts.UserID, ""); err != nil {
		logger.Warnf("AIP registration failed, using local ID: %v", err)
		agentID = c.generateAgentID(name)
	} else if id, ok := result["agent_id"].(string); ok && id != "" {
		agentID = id
	} else {
		agentID = c.generateAgentID(name)
	}

	identityMeta := map[string]any{}
	for k, v := range opts.Metadata {
		identityMeta[k] = v
	}
	identityMeta["endpoint_url"] = endpointURL
	identityMeta["mode"] = string(c.mode)
	if c.mode == ModeGateway {
		identityMeta["gateway_url"] = c.gatewayURL
		identityMeta["backend_url"] = c.agentBackendURL
	}

	identity := core.AgentIdentity{
		AgentID:       agentID,
		Name:          name,
		AgentType:     agentType,
		WalletAddress: opts.WalletAddress,
		Metadata:      identityMeta,
	}
	c.mu.Lock()
	c.identities[agentID] = identity
	c.mu.Unlock()

	logger.Infof("Agent registered successfully: %s (%s) in %s mode", agentID, name, c.mode)
	return identity, nil
}

// RegisterAgentInstance associates an in-process agent instance with an identity.
func (c *Client) RegisterAgentInstance(instance any, identity core.AgentIdentity) {
	c.mu.Lock()
	c.agents[identity.AgentID] = instance
	c.identities[identity.AgentID] = identity
	c.mu.Unlock()
}

// GetAgent returns a registered in-process agent instance, if any.
func (c *Client) GetAgent(agentID string) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agents[agentID]
}

// GetIdentity returns an agent's identity, querying the platform when not cached.
func (c *Client) GetIdentity(ctx context.Context, agentID string) (*core.AgentIdentity, error) {
	c.mu.Lock()
	if id, ok := c.identities[agentID]; ok {
		c.mu.Unlock()
		return &id, nil
	}
	c.mu.Unlock()
	return c.queryIdentityFromAIP(ctx, agentID)
}

// ListAgents lists locally tracked agents.
func (c *Client) ListAgents() []core.AgentIdentity {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]core.AgentIdentity, 0, len(c.identities))
	for _, id := range c.identities {
		out = append(out, id)
	}
	return out
}

// UpdateAgentMetadata merges metadata into a locally tracked identity.
func (c *Client) UpdateAgentMetadata(agentID string, metadata map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.identities[agentID]
	if !ok {
		return aiperr.AgentNotFound("", agentID)
	}
	if id.Metadata == nil {
		id.Metadata = map[string]any{}
	}
	for k, v := range metadata {
		id.Metadata[k] = v
	}
	c.identities[agentID] = id
	return nil
}

// RegisterAgentGroup registers an agent group with intelligent routing.
func (c *Client) RegisterAgentGroup(ctx context.Context, name, description string, memberAgentIDs []string, price float64, currency string, metadata map[string]any) (map[string]any, error) {
	if currency == "" {
		currency = "USD"
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	group := types.AgentGroupConfig{
		Name:           name,
		Description:    description,
		MemberAgentIDs: memberAgentIDs,
		Price:          price,
		Currency:       currency,
		Metadata:       metadata,
	}
	result, err := c.aip.RegisterAgentGroup(ctx, group)
	if err != nil {
		logger.Errorf("Group registration failed: %v", err)
		return nil, aiperr.Registry("Failed to register agent group: " + err.Error())
	}
	logger.Infof("Agent group registered: %v", result["group_id"])
	return result, nil
}

func (c *Client) generateAgentID(name string) string {
	unique := fmt.Sprintf("%s_%d_%s", name, time.Now().UnixNano(), uuid.NewString()[:8])
	sum := sha256.Sum256([]byte(unique))
	return "agent_" + hex.EncodeToString(sum[:])[:16]
}

func (c *Client) queryIdentityFromAIP(ctx context.Context, agentID string) (*core.AgentIdentity, error) {
	info, err := c.aip.GetAgent(ctx, "system", agentID)
	if err != nil || info == nil {
		return nil, err
	}
	return &core.AgentIdentity{
		AgentID:       info.AgentID,
		Name:          info.Name,
		AgentType:     core.AgentTypeAIP,
		WalletAddress: info.IdentityAddress,
		Metadata: map[string]any{
			"description":  info.Description,
			"handle":       info.Handle,
			"capabilities": info.Capabilities,
			"skills":       info.Skills,
			"endpoint_url": info.EndpointURL,
		},
	}, nil
}

// CheckA2AAgentHealth reports whether an A2A agent at the URL is healthy.
func (c *Client) CheckA2AAgentHealth(ctx context.Context, agentURL string) bool {
	return c.a2aClient.HealthCheck(ctx, agentURL)
}

// DiscoverA2AAgent discovers an external agent via the A2A protocol.
func (c *Client) DiscoverA2AAgent(ctx context.Context, agentURL string, forceRefresh, checkHealth bool) (*types.AgentCard, error) {
	if checkHealth && !c.a2aClient.HealthCheck(ctx, agentURL) {
		logger.Warnf("Agent at %s may not be healthy, proceeding anyway", agentURL)
	}
	card, err := c.a2aClient.DiscoverAgent(ctx, agentURL, forceRefresh)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.discoveredAgents[agentURL] = card
	c.mu.Unlock()
	logger.Infof("Discovered A2A Agent: %s at %s", card.Name, agentURL)
	return card, nil
}

// ListDiscoveredAgents lists all discovered A2A agents.
func (c *Client) ListDiscoveredAgents() []*types.AgentCard {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*types.AgentCard, 0, len(c.discoveredAgents))
	for _, card := range c.discoveredAgents {
		out = append(out, card)
	}
	return out
}

// SendA2ATask sends a task to an A2A agent.
func (c *Client) SendA2ATask(ctx context.Context, agentURL string, message *a2a.Message, taskID, contextID string) (*a2a.Task, error) {
	return c.a2aClient.SendTask(ctx, agentURL, message, taskID, contextID, nil)
}

// StreamA2ATask streams responses from an A2A agent.
func (c *Client) StreamA2ATask(ctx context.Context, agentURL string, message *a2a.Message, taskID, contextID string) <-chan a2a.ClientStreamResult {
	return c.a2aClient.StreamTask(ctx, agentURL, message, taskID, contextID)
}

// GetA2ATask gets an A2A task's status.
func (c *Client) GetA2ATask(ctx context.Context, agentURL, taskID string) (*a2a.Task, error) {
	return c.a2aClient.GetTask(ctx, agentURL, taskID)
}

// CancelA2ATask cancels an A2A task.
func (c *Client) CancelA2ATask(ctx context.Context, agentURL, taskID string) (*a2a.Task, error) {
	return c.a2aClient.CancelTask(ctx, agentURL, taskID)
}

// GenerateAgentCardFor generates an A2A agent card for a locally tracked agent.
func (c *Client) GenerateAgentCardFor(agentID, baseURL string, opts a2a.GenerateCardOptions) (*types.AgentCard, error) {
	c.mu.Lock()
	identity, ok := c.identities[agentID]
	c.mu.Unlock()
	if !ok {
		return nil, aiperr.AgentNotFound("", agentID)
	}
	card := a2a.GenerateAgentCard(identity, baseURL, opts)
	return &card, nil
}
