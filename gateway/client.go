// Package gateway provides clients for the Agent Gateway, mirroring
// aip_sdk/gateway/client.py and a2a_client.py.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultGatewayURL returns the URL from GATEWAY_URL, or the local default.
func DefaultGatewayURL() string {
	if url := os.Getenv("GATEWAY_URL"); url != "" {
		return strings.TrimRight(url, "/")
	}
	return "http://localhost:8080"
}

// Client interacts with the Agent Gateway.
type Client struct {
	gatewayURL string
	timeout    time.Duration
	http       *http.Client
}

// NewClient creates a gateway Client. An empty url falls back to DefaultGatewayURL.
func NewClient(gatewayURL string, timeout time.Duration) *Client {
	if gatewayURL == "" {
		gatewayURL = DefaultGatewayURL()
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		gatewayURL: strings.TrimRight(gatewayURL, "/"),
		timeout:    timeout,
		http:       &http.Client{Timeout: timeout},
	}
}

// GatewayURL returns the configured gateway URL.
func (c *Client) GatewayURL() string { return c.gatewayURL }

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.gatewayURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway error: HTTP %d: %s", resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// RegisterAgent registers an agent with the gateway.
func (c *Client) RegisterAgent(ctx context.Context, handle, endpointURL string, metadata map[string]any, force bool) (map[string]any, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload := map[string]any{
		"handle":       handle,
		"endpoint_url": endpointURL,
		"metadata":     metadata,
		"force":        force,
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/gateway/register", payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UnregisterAgent unregisters an agent from the gateway.
func (c *Client) UnregisterAgent(ctx context.Context, handle string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(ctx, http.MethodDelete, "/gateway/unregister/"+handle, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAgents lists all registered agents.
func (c *Client) ListAgents(ctx context.Context) ([]map[string]any, error) {
	var out struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := c.do(ctx, http.MethodGet, "/gateway/agents", nil, &out); err != nil {
		return nil, err
	}
	return out.Agents, nil
}

// GetAgentInfo returns information about a specific agent.
func (c *Client) GetAgentInfo(ctx context.Context, handle string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(ctx, http.MethodGet, "/gateway/agents/"+handle, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// HealthCheck reports whether the gateway is reachable and healthy.
func (c *Client) HealthCheck(ctx context.Context) bool {
	var out struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodGet, "/gateway/health", nil, &out); err != nil {
		return false
	}
	return out.Status == "healthy"
}

// WaitForGateway polls HealthCheck until available or attempts are exhausted.
func (c *Client) WaitForGateway(ctx context.Context, maxAttempts int, interval time.Duration) bool {
	for i := 0; i < maxAttempts; i++ {
		if c.HealthCheck(ctx) {
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
