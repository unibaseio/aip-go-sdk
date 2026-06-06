// Package platform provides the client for interacting with the AIP platform,
// mirroring aip_sdk/platform/client.py. The Python SDK exposes both an async
// and a sync client; in Go a single context-aware Client serves both roles.
package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/unibaseio/aip-go-sdk/aiperr"
	"github.com/unibaseio/aip-go-sdk/types"
)

// DefaultBaseURL returns the base URL from AIP_ENDPOINT, or the local default.
func DefaultBaseURL() string {
	if url := os.Getenv("AIP_ENDPOINT"); url != "" {
		return strings.TrimRight(url, "/")
	}
	return "http://localhost:8001"
}

// Config configures the AIP client.
type Config struct {
	BaseURL       string
	Timeout       time.Duration
	StreamTimeout time.Duration
	MaxRetries    int
	RetryDelay    time.Duration
	Headers       map[string]string
}

// Client is a client for the AIP platform.
type Client struct {
	cfg  Config
	http *http.Client
}

// Option customizes a Client.
type Option func(*Config)

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option { return func(c *Config) { c.Timeout = d } }

// WithStreamTimeout sets the streaming request timeout.
func WithStreamTimeout(d time.Duration) Option { return func(c *Config) { c.StreamTimeout = d } }

// WithHeaders sets default headers sent with every request.
func WithHeaders(h map[string]string) Option { return func(c *Config) { c.Headers = h } }

// New creates a Client. An empty baseURL falls back to DefaultBaseURL.
func New(baseURL string, opts ...Option) *Client {
	cfg := Config{
		BaseURL:       baseURL,
		Timeout:       60 * time.Second,
		StreamTimeout: 300 * time.Second,
		MaxRetries:    3,
		RetryDelay:    time.Second,
		Headers:       map[string]string{},
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL()
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string { return c.cfg.BaseURL }

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any, headers map[string]string) (*http.Request, error) {
	u := c.cfg.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// do executes a JSON request and decodes the response into out (if non-nil).
// Network errors and retryable status codes (429, 5xx) are retried up to
// MaxRetries times with linear backoff (RetryDelay × attempt), honoring ctx.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, headers map[string]string, out any) error {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.cfg.RetryDelay * time.Duration(attempt)):
			}
		}

		req, err := c.newRequest(ctx, method, path, query, body, headers)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = aiperr.Connection(err.Error(), c.cfg.BaseURL+path)
			continue // network errors are retryable
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = aiperr.New(fmt.Sprintf("HTTP %d for %s %s: %s", resp.StatusCode, method, path, string(data)), "", map[string]any{"status": resp.StatusCode})
			continue // 429 / 5xx are retryable
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return aiperr.New(fmt.Sprintf("HTTP %d for %s %s: %s", resp.StatusCode, method, path, string(data)), "", map[string]any{"status": resp.StatusCode})
		}
		if out != nil && len(data) > 0 {
			return json.Unmarshal(data, out)
		}
		return nil
	}
	return lastErr
}

// HealthCheck reports whether the AIP platform is healthy.
func (c *Client) HealthCheck(ctx context.Context) bool {
	req, err := c.newRequest(ctx, http.MethodGet, "/healthz", nil, nil, nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// WaitForReady polls HealthCheck until ready or attempts are exhausted.
func (c *Client) WaitForReady(ctx context.Context, maxAttempts int, interval time.Duration) bool {
	for i := 0; i < maxAttempts; i++ {
		if c.HealthCheck(ctx) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
	}
	return false
}

// ListUserAgents lists agents owned by a user with pagination.
func (c *Client) ListUserAgents(ctx context.Context, userID string, limit, offset int) (*types.PaginatedResponse, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var data struct {
		Agents []map[string]any `json:"agents"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/"+userID+"/agents", q, nil, nil, &data); err != nil {
		return nil, err
	}
	items := make([]any, 0, len(data.Agents))
	for _, a := range data.Agents {
		items = append(items, types.AgentInfoFromMap(a))
	}
	return &types.PaginatedResponse{Items: items, Total: data.Total, Limit: data.Limit, Offset: data.Offset}, nil
}

// GetAgent returns a specific agent owned by a user, or nil if not found.
func (c *Client) GetAgent(ctx context.Context, userID, agentID string) (*types.AgentInfo, error) {
	resp, err := c.ListUserAgents(ctx, userID, 1000, 0)
	if err != nil {
		return nil, err
	}
	for _, item := range resp.Items {
		if info, ok := item.(types.AgentInfo); ok && info.AgentID == agentID {
			return &info, nil
		}
	}
	return nil, nil
}

// RegisterAgent registers an agent with the AIP platform. When privyToken is
// set it is sent as a Bearer token and userID is omitted from the body.
func (c *Client) RegisterAgent(ctx context.Context, cfg types.AgentConfig, userID, privyToken string) (map[string]any, error) {
	regData := cfg.ToRegistrationMap()
	if userID != "" && privyToken == "" {
		regData["user_id"] = userID
	}
	var headers map[string]string
	if privyToken != "" {
		headers = map[string]string{"Authorization": "Bearer " + privyToken}
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/agents/register", nil, regData, headers, &out); err != nil {
		return nil, aiperr.Registration("Failed to register agent: "+err.Error(), "", fmt.Sprintf("%v", regData["handle"]))
	}
	return out, nil
}

// UnregisterAgent unregisters an agent owned by a user.
func (c *Client) UnregisterAgent(ctx context.Context, userID, agentID string) (map[string]any, error) {
	var out map[string]any
	if err := c.do(ctx, http.MethodDelete, "/users/"+userID+"/agents/"+agentID, nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RegisterAgentGroup registers an agent group.
func (c *Client) RegisterAgentGroup(ctx context.Context, group types.AgentGroupConfig) (map[string]any, error) {
	regData := group.ToRegistrationMap()
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/agents/groups/register", nil, regData, nil, &out); err != nil {
		return nil, aiperr.Registration("Failed to register agent group: "+err.Error(), "", fmt.Sprintf("%v", regData["group_name"]))
	}
	return out, nil
}

// RunOptions configure a run/run-stream call.
type RunOptions struct {
	Agent      string
	DomainHint string
	UserID     string
	Timeout    time.Duration
}

// StreamEvent carries a streamed EventData or a terminal error.
type StreamEvent struct {
	Event types.EventData
	Err   error
}

// RunStream executes a task and streams events on the returned channel. The
// channel is closed when the stream ends; a non-nil Err marks a failure.
func (c *Client) RunStream(ctx context.Context, objective string, opts RunOptions) <-chan StreamEvent {
	out := make(chan StreamEvent)
	go func() {
		defer close(out)

		payload := map[string]any{"objective": objective}
		if opts.Agent != "" {
			payload["agent"] = opts.Agent
		}
		if opts.DomainHint != "" {
			payload["domain_hint"] = opts.DomainHint
		}
		if opts.UserID != "" {
			payload["user_id"] = opts.UserID
		}

		streamTimeout := opts.Timeout
		if streamTimeout == 0 {
			streamTimeout = c.cfg.StreamTimeout
		}
		streamCtx, cancel := context.WithTimeout(ctx, streamTimeout)
		defer cancel()

		req, err := c.newRequest(streamCtx, http.MethodPost, "/runs/stream", nil, payload, nil)
		if err != nil {
			out <- StreamEvent{Err: err}
			return
		}
		resp, err := c.http.Do(req)
		if err != nil {
			out <- StreamEvent{Err: aiperr.Execution("Task execution failed: "+err.Error(), "", "", "")}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			out <- StreamEvent{Err: aiperr.Execution(fmt.Sprintf("Task execution failed: HTTP %d: %s", resp.StatusCode, string(body)), "", "", "")}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			raw := strings.TrimSpace(line[6:])
			if raw == "" {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				continue
			}
			out <- StreamEvent{Event: eventFromMap(data)}
		}
		if err := scanner.Err(); err != nil {
			out <- StreamEvent{Err: aiperr.Execution("Task execution failed: "+err.Error(), "", "", "")}
		}
	}()
	return out
}

func eventFromMap(data map[string]any) types.EventData {
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := data[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	eventType := getStr("eventType", "type")
	if eventType == "" {
		eventType = "unknown"
	}
	payload, ok := data["payload"].(map[string]any)
	if !ok {
		payload = data
	}
	return types.EventData{
		EventType: eventType,
		Payload:   payload,
		Timestamp: getStr("timestamp"),
		RunID:     getStr("runId"),
	}
}

// Run executes a task and returns the aggregated final result.
func (c *Client) Run(ctx context.Context, objective string, opts RunOptions) (*types.RunResult, error) {
	var events []types.EventData
	var payments []map[string]any
	var result map[string]any
	var runID, errMsg string

	for ev := range c.RunStream(ctx, objective, opts) {
		if ev.Err != nil {
			errMsg = ev.Err.Error()
			break
		}
		e := ev.Event
		events = append(events, e)
		if runID == "" {
			runID = e.RunID
		}
		if strings.Contains(strings.ToLower(e.EventType), "payment") {
			pd := map[string]any{"event_type": e.EventType, "timestamp": e.Timestamp}
			for k, v := range e.Payload {
				pd[k] = v
			}
			payments = append(payments, pd)
		}
		if e.IsCompleted() {
			result = e.Payload
		} else if e.IsError() {
			if m := e.Message(); m != "" {
				errMsg = m
			} else {
				errMsg = fmt.Sprintf("%v", e.Payload)
			}
		}
	}

	status := types.RunStatusCompleted
	if errMsg != "" {
		status = types.RunStatusFailed
	}
	return &types.RunResult{
		RunID:    runID,
		Status:   status,
		Result:   result,
		Events:   events,
		Error:    errMsg,
		Payments: payments,
	}, nil
}

// ListUsers lists registered users with pagination.
func (c *Client) ListUsers(ctx context.Context, limit, offset int) (*types.PaginatedResponse, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var data struct {
		Users  []types.UserInfo `json:"users"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/users", q, nil, nil, &data); err != nil {
		return nil, err
	}
	items := make([]any, 0, len(data.Users))
	for _, u := range data.Users {
		items = append(items, u)
	}
	return &types.PaginatedResponse{Items: items, Total: data.Total, Limit: data.Limit, Offset: data.Offset}, nil
}

// RegisterUser registers a new user. When privateKey is set the key-based
// endpoint is used.
func (c *Client) RegisterUser(ctx context.Context, walletAddress, email, privateKey string, chainID int) (map[string]any, error) {
	var out map[string]any
	if privateKey != "" {
		if chainID == 0 {
			chainID = 97
		}
		body := map[string]any{"wallet_address": walletAddress, "private_key": privateKey, "email": email, "chain_id": chainID}
		if err := c.do(ctx, http.MethodPost, "/accounts/users/register-with-key", nil, body, nil, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	body := map[string]any{"wallet_address": walletAddress, "email": email}
	if err := c.do(ctx, http.MethodPost, "/accounts/users/register", nil, body, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAgentPrice returns pricing for an agent owned by a user.
func (c *Client) GetAgentPrice(ctx context.Context, userID, agentID string) (*types.PriceInfo, error) {
	var out types.PriceInfo
	if err := c.do(ctx, http.MethodGet, "/users/"+userID+"/agents/"+agentID+"/pricing", nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAgentPrice updates pricing for an agent owned by a user.
func (c *Client) UpdateAgentPrice(ctx context.Context, userID, agentID string, amount float64, currency string, metadata map[string]any) (*types.PriceInfo, error) {
	payload := map[string]any{"identifier": agentID, "amount": amount}
	if currency != "" {
		payload["currency"] = currency
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}
	var out types.PriceInfo
	if err := c.do(ctx, http.MethodPut, "/users/"+userID+"/agents/"+agentID+"/pricing", nil, payload, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAgentPrices lists agent prices with pagination.
func (c *Client) ListAgentPrices(ctx context.Context, limit, offset int) (*types.PaginatedResponse, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var data struct {
		Prices []types.PriceInfo `json:"prices"`
		Total  int               `json:"total"`
		Limit  int               `json:"limit"`
		Offset int               `json:"offset"`
	}
	if err := c.do(ctx, http.MethodGet, "/pricing/agents", q, nil, nil, &data); err != nil {
		return nil, err
	}
	items := make([]any, 0, len(data.Prices))
	for _, p := range data.Prices {
		items = append(items, p)
	}
	return &types.PaginatedResponse{Items: items, Total: data.Total, Limit: data.Limit, Offset: data.Offset}, nil
}

// ListUserRuns lists runs for a user with pagination.
func (c *Client) ListUserRuns(ctx context.Context, userID string, limit, offset int) (*types.PaginatedResponse, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var data struct {
		Runs   []any `json:"runs"`
		Total  int   `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/"+userID+"/runs", q, nil, nil, &data); err != nil {
		return nil, err
	}
	return &types.PaginatedResponse{Items: data.Runs, Total: data.Total, Limit: data.Limit, Offset: data.Offset}, nil
}

// GetRunEvents returns the events for a run.
func (c *Client) GetRunEvents(ctx context.Context, runID string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.do(ctx, http.MethodGet, "/runs/"+runID+"/events", nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRunPayments returns the payments for a run.
func (c *Client) GetRunPayments(ctx context.Context, runID string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.do(ctx, http.MethodGet, "/runs/"+runID+"/payments", nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
