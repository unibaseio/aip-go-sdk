package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	a2ago "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"

	"github.com/unibaseio/aip-go-sdk/types"
)

// TaskExecutionError indicates a failure executing or fetching a task.
type TaskExecutionError struct {
	Message string
}

func (e *TaskExecutionError) Error() string { return e.Message }

// Client communicates with A2A-compliant agents. Protocol calls are delegated
// to the official a2a-go JSON-RPC client; agent-card discovery is done over the
// well-known endpoint and decoded into the Unibase ERC-8004 AgentCard.
type Client struct {
	timeout time.Duration
	headers map[string]string
	http    *http.Client

	mu      sync.Mutex
	clients map[string]*a2aclient.Client
	cache   map[string]*types.AgentCard
}

// NewClient creates an A2A client.
func NewClient(timeout time.Duration, headers map[string]string) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if headers == nil {
		headers = map[string]string{}
	}
	return &Client{
		timeout: timeout,
		headers: headers,
		http:    &http.Client{Timeout: timeout},
		clients: map[string]*a2aclient.Client{},
		cache:   map[string]*types.AgentCard{},
	}
}

func a2aEndpoint(agentURL string) string {
	agentURL = strings.TrimRight(agentURL, "/")
	if !strings.HasSuffix(agentURL, "/a2a") {
		agentURL += "/a2a"
	}
	return agentURL
}

// protocolClient returns (and caches) an a2a-go JSON-RPC client for the agent.
func (c *Client) protocolClient(ctx context.Context, agentURL string) (*a2aclient.Client, error) {
	endpoint := a2aEndpoint(agentURL)
	c.mu.Lock()
	if cl, ok := c.clients[endpoint]; ok {
		c.mu.Unlock()
		return cl, nil
	}
	c.mu.Unlock()

	cl, err := a2aclient.NewFromEndpoints(ctx,
		[]a2ago.AgentInterface{{Transport: a2ago.TransportProtocolJSONRPC, URL: endpoint}},
		a2aclient.WithJSONRPCTransport(c.http),
	)
	if err != nil {
		return nil, &TaskExecutionError{Message: "failed to create A2A client: " + err.Error()}
	}
	c.mu.Lock()
	c.clients[endpoint] = cl
	c.mu.Unlock()
	return cl, nil
}

// HealthCheck reports whether a remote agent is healthy, trying /health and
// /healthz, then falling back to agent-card discovery.
func (c *Client) HealthCheck(ctx context.Context, agentURL string) bool {
	base := strings.TrimRight(agentURL, "/")
	for _, ep := range []string{"/health", "/healthz"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+ep, nil)
		if err != nil {
			continue
		}
		c.applyHeaders(req)
		resp, err := c.http.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return true
		}
	}
	if _, err := c.DiscoverAgent(ctx, base, true); err == nil {
		return true
	}
	return false
}

func (c *Client) applyHeaders(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

// DiscoverAgent fetches (and caches) an agent's ERC-8004 card from its
// well-known URL.
func (c *Client) DiscoverAgent(ctx context.Context, baseURL string, forceRefresh bool) (*types.AgentCard, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if !forceRefresh {
		c.mu.Lock()
		if card, ok := c.cache[baseURL]; ok {
			c.mu.Unlock()
			return card, nil
		}
		c.mu.Unlock()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/.well-known/agent-card.json", nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &TaskExecutionError{Message: fmt.Sprintf("Failed to discover agent at %s: %v", baseURL, err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &TaskExecutionError{Message: fmt.Sprintf("Failed to discover agent at %s: HTTP %d", baseURL, resp.StatusCode)}
	}
	data, _ := io.ReadAll(resp.Body)
	var card types.AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, &TaskExecutionError{Message: fmt.Sprintf("Invalid agent card at %s: %v", baseURL, err)}
	}
	c.mu.Lock()
	c.cache[baseURL] = &card
	c.mu.Unlock()
	return &card, nil
}

// SendTask sends a task to a remote agent and returns the resulting Task. When
// the agent replies with a bare Message it is wrapped into a completed Task.
func (c *Client) SendTask(ctx context.Context, agentURL string, message *Message, taskID, contextID string, metadata map[string]any) (*Task, error) {
	cl, err := c.protocolClient(ctx, agentURL)
	if err != nil {
		return nil, err
	}
	applyIDs(message, taskID, contextID)
	result, err := cl.SendMessage(ctx, &a2ago.MessageSendParams{Message: message, Metadata: metadata})
	if err != nil {
		return nil, &TaskExecutionError{Message: "Task execution failed: " + err.Error()}
	}
	switch v := result.(type) {
	case *Task:
		return v, nil
	case *Message:
		return &Task{
			ID:        TaskID(taskID),
			ContextID: contextID,
			Status:    TaskStatus{State: TaskStateCompleted},
			History:   []*Message{v},
		}, nil
	default:
		return nil, &TaskExecutionError{Message: fmt.Sprintf("unexpected result type %T", v)}
	}
}

// GetTask fetches a task by ID from a remote agent.
func (c *Client) GetTask(ctx context.Context, agentURL, taskID string) (*Task, error) {
	cl, err := c.protocolClient(ctx, agentURL)
	if err != nil {
		return nil, err
	}
	task, err := cl.GetTask(ctx, &a2ago.TaskQueryParams{ID: TaskID(taskID)})
	if err != nil {
		return nil, &TaskExecutionError{Message: "Get task failed: " + err.Error()}
	}
	return task, nil
}

// CancelTask cancels a task on a remote agent.
func (c *Client) CancelTask(ctx context.Context, agentURL, taskID string) (*Task, error) {
	cl, err := c.protocolClient(ctx, agentURL)
	if err != nil {
		return nil, err
	}
	task, err := cl.CancelTask(ctx, &a2ago.TaskIDParams{ID: TaskID(taskID)})
	if err != nil {
		return nil, &TaskExecutionError{Message: "Cancel task failed: " + err.Error()}
	}
	return task, nil
}

// ClientStreamResult carries a streamed response or a terminal error.
type ClientStreamResult struct {
	Response *StreamResponse
	Err      error
}

// StreamTask streams task responses from a remote agent.
func (c *Client) StreamTask(ctx context.Context, agentURL string, message *Message, taskID, contextID string) <-chan ClientStreamResult {
	out := make(chan ClientStreamResult)
	go func() {
		defer close(out)
		cl, err := c.protocolClient(ctx, agentURL)
		if err != nil {
			out <- ClientStreamResult{Err: err}
			return
		}
		applyIDs(message, taskID, contextID)
		for ev, err := range cl.SendStreamingMessage(ctx, &a2ago.MessageSendParams{Message: message}) {
			if err != nil {
				out <- ClientStreamResult{Err: &TaskExecutionError{Message: "Stream error: " + err.Error()}}
				return
			}
			out <- ClientStreamResult{Response: eventToStreamResponse(ev)}
		}
	}()
	return out
}

func applyIDs(message *Message, taskID, contextID string) {
	if taskID != "" {
		message.TaskID = TaskID(taskID)
	}
	if contextID != "" {
		message.ContextID = contextID
	}
}

func eventToStreamResponse(ev Event) *StreamResponse {
	switch v := ev.(type) {
	case *Task:
		return &StreamResponse{Task: v}
	case *Message:
		return &StreamResponse{Message: v}
	case *TaskStatusUpdateEvent:
		return &StreamResponse{StatusUpdate: v}
	case *TaskArtifactUpdateEvent:
		return &StreamResponse{ArtifactUpdate: v}
	default:
		return &StreamResponse{}
	}
}
