package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unibaseio/unibase-aip-sdk-go/internal/log"
)

var extLogger = log.Get("agent.external")

// TaskExecutor executes a task payload and returns a result. External agents
// implement this to plug their logic into ExternalAgentClient.
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, payload map[string]any) (map[string]any, error)
}

// TaskExecutorFunc adapts a function to the TaskExecutor interface.
type TaskExecutorFunc func(ctx context.Context, payload map[string]any) (map[string]any, error)

// ExecuteTask calls f.
func (f TaskExecutorFunc) ExecuteTask(ctx context.Context, payload map[string]any) (map[string]any, error) {
	return f(ctx, payload)
}

// ExternalAgentClient pulls tasks from a gateway and executes them via an
// Executor, mirroring aip_sdk/agent/external.py.
type ExternalAgentClient struct {
	AgentName         string
	GatewayURL        string
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	Capabilities      []string
	Metadata          map[string]any
	Executor          TaskExecutor

	http *http.Client

	mu             sync.Mutex
	running        bool
	agentID        string
	currentTaskID  string
	tasksCompleted int
	tasksFailed    int
	startedAt      time.Time
}

// NewExternalAgentClient creates an external agent client.
func NewExternalAgentClient(agentName, gatewayURL string, executor TaskExecutor) *ExternalAgentClient {
	return &ExternalAgentClient{
		AgentName:         agentName,
		GatewayURL:        strings.TrimRight(gatewayURL, "/"),
		PollInterval:      5 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		Metadata:          map[string]any{},
		Executor:          executor,
		http:              &http.Client{},
	}
}

func (c *ExternalAgentClient) post(ctx context.Context, path string, body any, timeout time.Duration, out any) error {
	b, _ := json.Marshal(body)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.GatewayURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{status: resp.StatusCode, body: string(data)}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string { return "HTTP " + strconv.Itoa(e.status) + ": " + e.body }

// Register registers the agent with the gateway, preferring the
// register-external endpoint and falling back to the standard register flow.
func (c *ExternalAgentClient) Register(ctx context.Context) error {
	meta := map[string]any{"sdk_version": "1.0.0"}
	for k, v := range c.Metadata {
		meta[k] = v
	}
	var data struct {
		AgentID   string `json:"agent_id"`
		AgentName string `json:"agent_name"`
	}
	err := c.post(ctx, "/gateway/agents/register-external", map[string]any{
		"handle":       c.AgentName,
		"agent_id":     c.agentID,
		"capabilities": c.Capabilities,
		"metadata":     meta,
	}, 15*time.Second, &data)
	if err == nil {
		c.agentID = data.AgentID
		extLogger.Infof("Agent '%s' registered successfully (ID: %s)", c.AgentName, c.agentID)
		return nil
	}
	extLogger.Warnf("register-external failed, falling back to standard register: %v", err)

	fallbackMeta := map[string]any{"mode": "external", "capabilities": c.Capabilities, "sdk_version": "1.0.0"}
	for k, v := range c.Metadata {
		fallbackMeta[k] = v
	}
	if err := c.post(ctx, "/gateway/register", map[string]any{
		"agent_name":  c.AgentName,
		"backend_url": "external",
		"metadata":    fallbackMeta,
		"force":       true,
	}, 10*time.Second, &data); err != nil {
		extLogger.Errorf("Registration failed: %v", err)
		return err
	}
	c.agentID = data.AgentName
	if c.agentID == "" {
		c.agentID = c.AgentName
	}
	extLogger.Infof("Agent '%s' registered via fallback (ID: %s)", c.AgentName, c.agentID)

	_ = c.post(ctx, "/gateway/agents/heartbeat", map[string]any{
		"handle":   c.AgentName,
		"agent_id": c.agentID,
		"status":   "idle",
		"metadata": map[string]any{"capabilities": c.Capabilities, "sdk_version": "1.0.0"},
	}, 10*time.Second, nil)
	return nil
}

// PollTask long-polls the gateway for the next task, returning nil when none.
func (c *ExternalAgentClient) PollTask(ctx context.Context, timeout time.Duration) map[string]any {
	q := url.Values{"agent": {c.AgentName}, "timeout": {strconv.FormatFloat(timeout.Seconds(), 'f', -1, 64)}}
	reqCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.GatewayURL+"/gateway/tasks/poll?"+q.Encode(), nil)
	if err != nil {
		return nil
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	var task map[string]any
	if json.Unmarshal(data, &task) != nil {
		return nil
	}
	if id, _ := task["task_id"].(string); id != "" {
		return task
	}
	return nil
}

// CompleteTask reports task completion (or failure) to the gateway.
func (c *ExternalAgentClient) CompleteTask(ctx context.Context, taskID string, result map[string]any, status, errMsg string, executionTime float64) {
	body := map[string]any{
		"task_id":        taskID,
		"status":         status,
		"result":         result,
		"error":          errMsg,
		"execution_time": executionTime,
	}
	if err := c.post(ctx, "/gateway/tasks/complete", body, 10*time.Second, nil); err != nil {
		extLogger.Errorf("Failed to report task completion: %v", err)
		return
	}
	extLogger.Infof("Task %s completed with status '%s'", taskID, status)
}

// SendHeartbeat sends a single heartbeat to the gateway.
func (c *ExternalAgentClient) SendHeartbeat(ctx context.Context) {
	c.mu.Lock()
	status := "idle"
	if c.currentTaskID != "" {
		status = "busy"
	}
	uptime := 0.0
	if !c.startedAt.IsZero() {
		uptime = time.Since(c.startedAt).Seconds()
	}
	body := map[string]any{
		"handle":       c.AgentName,
		"agent_id":     c.agentID,
		"status":       status,
		"current_task": c.currentTaskID,
		"metadata": map[string]any{
			"tasks_completed": c.tasksCompleted,
			"tasks_failed":    c.tasksFailed,
			"uptime":          uptime,
		},
	}
	c.mu.Unlock()
	if err := c.post(ctx, "/gateway/agents/heartbeat", body, 5*time.Second, nil); err != nil {
		extLogger.Errorf("Heartbeat failed: %v", err)
	}
}

// Run registers, then polls for and executes tasks until the context is canceled.
func (c *ExternalAgentClient) Run(ctx context.Context) error {
	if err := c.Register(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.running = true
	c.startedAt = time.Now()
	c.mu.Unlock()

	go c.heartbeatLoop(ctx)
	extLogger.Infof("Agent '%s' started, polling for tasks...", c.AgentName)

	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.running = false
			completed, failed := c.tasksCompleted, c.tasksFailed
			c.mu.Unlock()
			extLogger.Infof("Agent '%s' stopped. Completed: %d, Failed: %d", c.AgentName, completed, failed)
			return nil
		default:
		}

		task := c.PollTask(ctx, 30*time.Second)
		if task == nil {
			sleepCtx(ctx, c.PollInterval)
			continue
		}
		taskID, _ := task["task_id"].(string)
		payload, _ := task["payload"].(map[string]any)
		extLogger.Infof("Received task %s", taskID)

		c.mu.Lock()
		c.currentTaskID = taskID
		c.mu.Unlock()
		start := time.Now()

		result, err := c.Executor.ExecuteTask(ctx, payload)
		execTime := time.Since(start).Seconds()
		if err != nil {
			extLogger.Errorf("Task execution failed: %v", err)
			c.CompleteTask(ctx, taskID, map[string]any{"error": err.Error()}, "failed", err.Error(), execTime)
			c.mu.Lock()
			c.tasksFailed++
			c.mu.Unlock()
		} else {
			c.CompleteTask(ctx, taskID, result, "completed", "", execTime)
			c.mu.Lock()
			c.tasksCompleted++
			c.mu.Unlock()
		}
		c.mu.Lock()
		c.currentTaskID = ""
		c.mu.Unlock()
	}
}

func (c *ExternalAgentClient) heartbeatLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.HeartbeatInterval):
			c.SendHeartbeat(ctx)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
