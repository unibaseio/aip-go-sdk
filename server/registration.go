package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/aip-go-sdk/platform"
	"github.com/unibaseio/aip-go-sdk/types"
)

// registerWithAIP registers the agent with the AIP platform using the
// registration config. Failures are logged and do not abort startup.
func (s *Server) registerWithAIP(ctx context.Context) {
	cfg := s.registrationConfig
	if cfg == nil {
		return
	}
	endpoint := cfg.AIPEndpoint
	if endpoint == "" {
		endpoint = platform.DefaultBaseURL()
	}
	logger.Infof("Registering agent with AIP platform at %s (handle erc8004:%s)", endpoint, cfg.Handle)

	costModel := types.CostModel{}
	if cfg.CostModel != nil {
		costModel = *cfg.CostModel
	}
	currency := cfg.Currency
	if currency == "" {
		currency = "USD"
	}
	chainID := cfg.ChainID
	if chainID == 0 {
		chainID = 97
	}
	agentConfig := types.AgentConfig{
		Name:         cfg.Name,
		Handle:       cfg.Handle,
		Description:  cfg.Description,
		EndpointURL:  cfg.EndpointURL,
		Skills:       cfg.Skills,
		CostModel:    costModel,
		Currency:     currency,
		Metadata:     cfg.Metadata,
		JobOfferings: cfg.JobOfferings,
		JobResources: cfg.JobResources,
		ChainID:      chainID,
		Signature:    cfg.Signature,
		Message:      cfg.Message,
	}

	client := platform.New(endpoint)
	result, err := client.RegisterAgent(ctx, agentConfig, cfg.UserID, cfg.PrivyToken)
	if err != nil {
		logger.Warnf("AIP registration failed (agent will run without registration): %v", err)
		return
	}
	agentID, _ := result["agent_id"].(string)
	if agentID == "" {
		agentID = "erc8004:" + cfg.Handle
	}
	s.mu.Lock()
	s.agentID = agentID
	s.mu.Unlock()
	logger.Infof("Agent registered successfully: %s", agentID)
}

// gatewayPollingLoop polls the gateway for tasks (task queue) or jobs (job
// queue) for private agents behind a firewall, mirroring the Python loop.
func (s *Server) gatewayPollingLoop(ctx context.Context) {
	cfg := s.registrationConfig
	gatewayURL := cfg.GatewayURL
	handle := cfg.Handle
	agentIDForPoll := s.AgentID()
	if agentIDForPoll == "" {
		agentIDForPoll = handle
	}
	pollInterval := 3 * time.Second

	useJobQueue := len(cfg.JobOfferings) > 0 || cfg.ViaGateway
	var pollEndpoint, completeEndpoint, pollAgent string
	if useJobQueue {
		pollEndpoint = gatewayURL + "/gateway/jobs/poll"
		completeEndpoint = gatewayURL + "/gateway/jobs/complete"
		pollAgent = agentIDForPoll
		logger.Infof("Starting Gateway JOB-QUEUE polling loop for agent %s", agentIDForPoll)
	} else {
		pollEndpoint = gatewayURL + "/gateway/tasks/poll"
		completeEndpoint = gatewayURL + "/gateway/tasks/complete"
		pollAgent = handle
		logger.Infof("Starting Gateway TASK-QUEUE polling loop for agent %s", handle)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Gateway polling loop stopped")
			return
		default:
		}

		q := url.Values{"agent": {pollAgent}, "timeout": {"5.0"}}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pollEndpoint+"?"+q.Encode(), nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Errorf("Error in polling loop: %v", err)
			sleep(ctx, pollInterval)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			sleep(ctx, pollInterval)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var payload map[string]any
		_ = json.Unmarshal(data, &payload)

		taskID, _ := payload["task_id"].(string)
		if taskID == "" {
			taskID, _ = payload["job_id"].(string)
		}
		if taskID == "" {
			sleep(ctx, pollInterval)
			continue
		}
		logger.Infof("Received assignment %s from Gateway (job_queue=%v)", taskID, useJobQueue)
		if useJobQueue {
			s.processGatewayJob(ctx, httpClient, taskID, payload, completeEndpoint)
		} else {
			s.processGatewayTask(ctx, httpClient, taskID, payload, completeEndpoint)
		}
	}
}

func (s *Server) processGatewayJob(ctx context.Context, client *http.Client, jobID string, jobData map[string]any, completeEndpoint string) {
	jobInput, _ := jobData["job_input"].(string)
	params := map[string]any{
		"message": map[string]any{
			"messageId": uuid.NewString(),
			"role":      "user",
			"parts":     []map[string]any{{"kind": "text", "text": jobInput}},
		},
	}
	taskDict, err := s.handleMessageSend(ctx, params)
	if err != nil {
		taskDict = map[string]any{}
	}
	agentText := extractAgentText(taskDict)
	resultPayload := map[string]any{"response": agentText, "task": taskDict}

	body := map[string]any{
		"job_id":   jobID,
		"agent_id": jobData["agent_id"],
		"result":   resultPayload,
		"status":   "completed",
	}
	if err != nil {
		body["status"] = "failed"
		body["error"] = err.Error()
		body["result"] = map[string]any{}
	}
	postJSON(ctx, client, completeEndpoint, body)
	logger.Infof("Job %s completed and result submitted to job queue", jobID)
}

func (s *Server) processGatewayTask(ctx context.Context, client *http.Client, taskID string, taskData map[string]any, completeEndpoint string) {
	payload, _ := taskData["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}
	params, _ := payload["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	method, _ := payload["method"].(string)

	var result any
	var err error
	switch method {
	case "", "message/send":
		result, err = s.handleMessageSend(ctx, params)
	case "tasks/get":
		result, err = s.handleTasksGet(params)
	case "tasks/cancel":
		result, err = s.handleTasksCancel(params)
	default:
		result, err = s.handleMessageSend(ctx, params)
	}

	body := map[string]any{"task_id": taskID, "status": "completed", "result": result}
	if err != nil {
		body["status"] = "failed"
		body["error"] = err.Error()
		delete(body, "result")
	}
	postJSON(ctx, client, completeEndpoint, body)
	logger.Infof("Task %s completed and result submitted", taskID)
}

func postJSON(ctx context.Context, client *http.Client, urlStr string, body any) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("Failed to submit result: %v", err)
		return
	}
	resp.Body.Close()
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
