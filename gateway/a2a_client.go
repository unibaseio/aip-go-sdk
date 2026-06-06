package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/aip-go-sdk/a2a"
	"github.com/unibaseio/aip-go-sdk/agent"
	"github.com/unibaseio/aip-go-sdk/types"
)

// GatewayError is a gateway communication error.
type GatewayError struct {
	Message    string
	StatusCode int
}

func (e *GatewayError) Error() string { return e.Message }

// TaskTimeoutError indicates a task exceeded its polling deadline.
type TaskTimeoutError struct {
	TaskID  string
	Timeout time.Duration
}

func (e *TaskTimeoutError) Error() string {
	return fmt.Sprintf("Task %s timed out after %s", e.TaskID, e.Timeout)
}

// A2AMode selects how the gateway mediates communication.
type A2AMode string

const (
	// ModePush forwards the task to the agent and returns its response.
	ModePush A2AMode = "push"
	// ModePull queues the task for the agent to poll, then polls for the result.
	ModePull A2AMode = "pull"
)

// A2AClient performs gateway-mediated agent communication.
type A2AClient struct {
	gatewayURL   string
	mode         A2AMode
	timeout      time.Duration
	pollInterval time.Duration
	maxPollTime  time.Duration
	headers      map[string]string
	http         *http.Client

	mu           sync.Mutex
	agentCards   map[string]*types.AgentCard
	pendingTasks map[string]*a2a.Task
}

// A2AOption customizes an A2AClient.
type A2AOption func(*A2AClient)

// WithMode sets the mediation mode (push or pull).
func WithMode(m A2AMode) A2AOption { return func(c *A2AClient) { c.mode = m } }

// WithPollInterval sets the pull-mode polling interval.
func WithPollInterval(d time.Duration) A2AOption { return func(c *A2AClient) { c.pollInterval = d } }

// WithMaxPollTime sets the pull-mode polling deadline.
func WithMaxPollTime(d time.Duration) A2AOption { return func(c *A2AClient) { c.maxPollTime = d } }

// WithA2AHeaders sets default request headers.
func WithA2AHeaders(h map[string]string) A2AOption { return func(c *A2AClient) { c.headers = h } }

// WithA2ATimeout sets the per-request timeout.
func WithA2ATimeout(d time.Duration) A2AOption { return func(c *A2AClient) { c.timeout = d } }

// NewA2AClient creates a gateway A2A client.
func NewA2AClient(gatewayURL string, opts ...A2AOption) *A2AClient {
	c := &A2AClient{
		gatewayURL:   strings.TrimRight(gatewayURL, "/"),
		mode:         ModePush,
		timeout:      30 * time.Second,
		pollInterval: time.Second,
		maxPollTime:  300 * time.Second,
		headers:      map[string]string{},
		agentCards:   map[string]*types.AgentCard{},
		pendingTasks: map[string]*a2a.Task{},
	}
	for _, o := range opts {
		o(c)
	}
	c.http = &http.Client{Timeout: c.timeout}
	return c
}

// Mode returns the configured mediation mode.
func (c *A2AClient) Mode() A2AMode { return c.mode }

type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	ID      string         `json:"id"`
}

func newJSONRPC(method string, params map[string]any) jsonRPCRequest {
	return jsonRPCRequest{JSONRPC: "2.0", Method: method, Params: params, ID: uuid.NewString()}
}

func (c *A2AClient) applyHeaders(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

// do executes a gateway JSON request and decodes the response into out.
func (c *A2AClient) do(ctx context.Context, method, path string, body any, out any) error {
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
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &GatewayError{Message: fmt.Sprintf("gateway error: HTTP %d: %s", resp.StatusCode, string(data)), StatusCode: resp.StatusCode}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// SendTaskOptions configure a SendTask call.
type SendTaskOptions struct {
	TaskID     string
	ContextID  string
	AIPContext *agent.AIPContext
}

// SendTask sends a task through the gateway and returns the resulting Task.
// In pull mode it submits to the task queue and polls until completion.
func (c *A2AClient) SendTask(ctx context.Context, agentID string, message *a2a.Message, opts SendTaskOptions) (*a2a.Task, error) {
	if opts.AIPContext != nil {
		message = agent.WrapMessage(message, opts.AIPContext)
	}
	taskID := opts.TaskID
	if taskID == "" {
		taskID = uuid.NewString()
	}
	if c.mode == ModePush {
		return c.pushSend(ctx, agentID, message, taskID, opts.ContextID)
	}
	return c.pullSend(ctx, agentID, message, taskID, opts.ContextID)
}

func (c *A2AClient) pushSend(ctx context.Context, agentID string, message *a2a.Message, taskID, contextID string) (*a2a.Task, error) {
	params := map[string]any{"id": taskID, "message": message}
	if contextID != "" {
		params["contextId"] = contextID
	}
	if message.Metadata != nil {
		params["metadata"] = message.Metadata
	}
	reqBody := newJSONRPC("message/send", params)

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL+"/gateway/a2a/"+agentID, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &GatewayError{Message: "Request error: " + err.Error()}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &GatewayError{Message: fmt.Sprintf("Gateway error: %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, &GatewayError{Message: "Task failed: " + rpcResp.Error.Message, StatusCode: rpcResp.Error.Code}
	}
	var task a2a.Task
	if err := json.Unmarshal(rpcResp.Result, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// PushStream streams task events via push mode. The channel is closed when the
// stream ends; a StreamResult with a non-nil Err marks a failure.
func (c *A2AClient) PushStream(ctx context.Context, agentID string, message *a2a.Message, opts SendTaskOptions) <-chan StreamResult {
	out := make(chan StreamResult)
	go func() {
		defer close(out)
		if opts.AIPContext != nil {
			message = agent.WrapMessage(message, opts.AIPContext)
		}
		taskID := opts.TaskID
		if taskID == "" {
			taskID = uuid.NewString()
		}
		params := map[string]any{"id": taskID, "message": message}
		if opts.ContextID != "" {
			params["contextId"] = opts.ContextID
		}
		reqBody := newJSONRPC("message/stream", params)
		b, _ := json.Marshal(reqBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL+"/gateway/a2a/"+agentID+"/stream", bytes.NewReader(b))
		if err != nil {
			out <- StreamResult{Err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		c.applyHeaders(req)
		resp, err := c.http.Do(req)
		if err != nil {
			out <- StreamResult{Err: &GatewayError{Message: "Request error: " + err.Error()}}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			out <- StreamResult{Err: &GatewayError{Message: fmt.Sprintf("Gateway stream error: %d", resp.StatusCode), StatusCode: resp.StatusCode}}
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var data map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line[6:]), &data); err != nil {
				continue
			}
			if errRaw, ok := data["error"]; ok && string(errRaw) != "null" {
				var e struct {
					Message string `json:"message"`
				}
				_ = json.Unmarshal(errRaw, &e)
				out <- StreamResult{Err: &GatewayError{Message: "Stream error: " + e.Message}}
				return
			}
			if resultRaw, ok := data["result"]; ok {
				out <- StreamResult{Response: parseStreamResponse(resultRaw)}
			}
		}
	}()
	return out
}

// StreamResult carries a streamed response or a terminal error.
type StreamResult struct {
	Response *a2a.StreamResponse
	Err      error
}

func parseStreamResponse(raw json.RawMessage) *a2a.StreamResponse {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return &a2a.StreamResponse{}
	}
	resp := &a2a.StreamResponse{}
	if t, ok := data["task"]; ok {
		var task a2a.Task
		if json.Unmarshal(t, &task) == nil {
			resp.Task = &task
		}
	} else if m, ok := data["message"]; ok {
		var msg a2a.Message
		if json.Unmarshal(m, &msg) == nil {
			resp.Message = &msg
		}
	} else if su, ok := data["statusUpdate"]; ok {
		var ev a2a.TaskStatusUpdateEvent
		if json.Unmarshal(su, &ev) == nil {
			resp.StatusUpdate = &ev
		}
	} else if au, ok := data["artifactUpdate"]; ok {
		var ev a2a.TaskArtifactUpdateEvent
		if json.Unmarshal(au, &ev) == nil {
			resp.ArtifactUpdate = &ev
		}
	}
	return resp
}

func (c *A2AClient) pullSend(ctx context.Context, agentID string, message *a2a.Message, taskID, contextID string) (*a2a.Task, error) {
	ctxID := contextID
	if ctxID == "" {
		ctxID = taskID
	}
	task := &a2a.Task{
		ID:        a2a.TaskID(taskID),
		ContextID: ctxID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateSubmitted},
		History:   []*a2a.Message{message},
	}
	c.mu.Lock()
	c.pendingTasks[taskID] = task
	c.mu.Unlock()

	agentHandle := agentID
	if i := strings.LastIndex(agentID, ":"); i >= 0 {
		agentHandle = agentID[i+1:]
	}

	jsonrpc := map[string]any{
		"jsonrpc": "2.0",
		"method":  "message/send",
		"params":  map[string]any{"message": message, "id": taskID, "contextId": contextID},
		"id":      taskID,
	}
	submit := map[string]any{"task_id": taskID, "agent": agentHandle, "payload": jsonrpc}

	if err := c.do(ctx, http.MethodPost, "/gateway/tasks/submit", submit, nil); err != nil {
		task.Status = a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), err.Error())}
		return task, nil
	}

	task.Status = a2a.TaskStatus{State: a2a.TaskStateWorking}
	deadline := time.Now().Add(c.maxPollTime)
	for {
		if time.Now().After(deadline) {
			task.Status = a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), "Task timed out")}
			return task, &TaskTimeoutError{TaskID: taskID, Timeout: c.maxPollTime}
		}
		select {
		case <-ctx.Done():
			return task, ctx.Err()
		case <-time.After(c.pollInterval):
		}

		var statusResp struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := c.do(ctx, http.MethodGet, "/gateway/tasks/"+taskID+"/status", nil, &statusResp); err != nil {
			continue
		}
		switch statusResp.Status {
		case "completed":
			var resultData map[string]json.RawMessage
			if err := c.do(ctx, http.MethodGet, "/gateway/tasks/"+taskID+"/result", nil, &resultData); err == nil {
				task.Status = a2a.TaskStatus{State: a2a.TaskStateCompleted}
				if r, ok := resultData["result"]; ok {
					applyResult(task, r)
				}
			}
			c.mu.Lock()
			delete(c.pendingTasks, taskID)
			c.mu.Unlock()
			return task, nil
		case "failed":
			task.Status = a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), statusResp.Error)}
			c.mu.Lock()
			delete(c.pendingTasks, taskID)
			c.mu.Unlock()
			return task, nil
		case "canceled":
			task.Status = a2a.TaskStatus{State: a2a.TaskStateCanceled}
			c.mu.Lock()
			delete(c.pendingTasks, taskID)
			c.mu.Unlock()
			return task, nil
		}
	}
}

// applyResult merges an agent's result (a JSON-RPC response or Task) into task.
func applyResult(task *a2a.Task, raw json.RawMessage) {
	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	taskData := raw
	if json.Unmarshal(raw, &wrapper) == nil && len(wrapper.Result) > 0 {
		taskData = wrapper.Result
	}
	var parsed struct {
		History   []*a2a.Message  `json:"history"`
		Artifacts []*a2a.Artifact `json:"artifacts"`
	}
	if json.Unmarshal(taskData, &parsed) != nil {
		return
	}
	for _, msg := range parsed.History {
		if msg != nil && msg.Role == a2a.RoleAgent {
			task.History = append(task.History, msg)
		}
	}
	task.Artifacts = append(task.Artifacts, parsed.Artifacts...)
}

// GetAgentCard fetches (and caches) an agent's card via the gateway.
func (c *A2AClient) GetAgentCard(ctx context.Context, agentID string) (*types.AgentCard, error) {
	c.mu.Lock()
	if card, ok := c.agentCards[agentID]; ok {
		c.mu.Unlock()
		return card, nil
	}
	c.mu.Unlock()

	var card types.AgentCard
	if err := c.do(ctx, http.MethodGet, "/gateway/agents/"+agentID+"/card", nil, &card); err != nil {
		return nil, nil
	}
	c.mu.Lock()
	c.agentCards[agentID] = &card
	c.mu.Unlock()
	return &card, nil
}

// CancelTask requests cancellation of a task via the gateway.
func (c *A2AClient) CancelTask(ctx context.Context, agentID, taskID string) bool {
	if err := c.do(ctx, http.MethodPost, "/gateway/tasks/"+taskID+"/cancel", map[string]any{"agent": agentID}, nil); err != nil {
		return false
	}
	c.mu.Lock()
	if t, ok := c.pendingTasks[taskID]; ok {
		t.Status = a2a.TaskStatus{State: a2a.TaskStateCanceled}
	}
	c.mu.Unlock()
	return true
}

// GetTask returns the current state of a task, checking local cache first.
func (c *A2AClient) GetTask(ctx context.Context, agentID, taskID string) (*a2a.Task, error) {
	c.mu.Lock()
	if t, ok := c.pendingTasks[taskID]; ok {
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()

	var task a2a.Task
	if err := c.do(ctx, http.MethodGet, "/gateway/tasks/"+taskID, nil, &task); err != nil {
		return nil, nil
	}
	return &task, nil
}

// DiscoverAgent fetches an agent card from a well-known endpoint URL.
func (c *A2AClient) DiscoverAgent(ctx context.Context, endpointURL string) (*types.AgentCard, error) {
	endpointURL = strings.TrimRight(endpointURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL+"/.well-known/agent-card.json", nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	data, _ := io.ReadAll(resp.Body)
	var card types.AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, nil
	}
	c.mu.Lock()
	c.agentCards[card.Name] = &card
	c.mu.Unlock()
	return &card, nil
}
