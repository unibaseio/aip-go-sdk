// Package server provides an A2A-compliant HTTP server for exposing Unibase
// agents, mirroring aip_sdk/a2a/server.py. The Python server is built on
// FastAPI/uvicorn; this implementation uses net/http.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/aip-go-sdk/a2a"
	"github.com/unibaseio/aip-go-sdk/internal/log"
	"github.com/unibaseio/aip-go-sdk/types"
)

var logger = log.Get("a2a.server")

// TaskHandler processes a task and streams responses on the returned channel.
// The channel must be closed when handling is complete.
type TaskHandler func(ctx context.Context, task *a2a.Task, message *a2a.Message) <-chan a2a.StreamResponse

// RegistrationConfig holds the platform/gateway registration details that
// drive auto-registration and gateway polling.
type RegistrationConfig struct {
	Handle       string
	Name         string
	Description  string
	UserID       string
	PrivyToken   string
	AIPEndpoint  string
	EndpointURL  string
	GatewayURL   string
	ViaGateway   bool
	ChainID      int
	Currency     string
	Skills       []types.SkillConfig
	CostModel    *types.CostModel
	Metadata     map[string]any
	JobOfferings []types.AgentJobOffering
	JobResources []types.AgentJobResource
}

// Server is an A2A-compliant server exposing a single agent.
type Server struct {
	AgentCard          types.AgentCard
	handler            TaskHandler
	host               string
	port               int
	registrationConfig *RegistrationConfig
	autoRegister       bool

	mu      sync.Mutex
	tasks   map[string]*a2a.Task
	agentID string

	pollCancel context.CancelFunc
}

// Option customizes a Server.
type Option func(*Server)

// WithRegistration attaches a registration config; auto-register controls
// whether the agent registers with the platform on start.
func WithRegistration(cfg *RegistrationConfig, autoRegister bool) Option {
	return func(s *Server) {
		s.registrationConfig = cfg
		s.autoRegister = autoRegister
	}
}

// New creates an A2A Server for the given agent card and task handler.
func New(card types.AgentCard, handler TaskHandler, host string, port int, opts ...Option) *Server {
	if host == "" {
		host = "0.0.0.0"
	}
	if port == 0 {
		port = 8000
	}
	s := &Server{
		AgentCard:    card,
		handler:      handler,
		host:         host,
		port:         port,
		autoRegister: true,
		tasks:        map[string]*a2a.Task{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// AgentID returns the registered agent ID, if registration has occurred.
func (s *Server) AgentID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentID
}

// Handler builds the HTTP handler with all A2A routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent-card.json", s.handleAgentCard)
	mux.HandleFunc("POST /", s.handleJSONRPC)
	mux.HandleFunc("POST /a2a", s.handleJSONRPC)
	mux.HandleFunc("POST /a2a/stream", s.handleStream)
	mux.HandleFunc("POST /invoke", s.handleInvoke)
	mux.HandleFunc("POST /invoke/stream", s.handleInvokeStream)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /conversations", s.handleConversations)
	mux.HandleFunc("GET /conversations/{conversation_id}", s.handleConversation)
	return corsMiddleware(mux)
}

// Run starts the server, performing auto-registration and gateway polling as
// configured, and blocks until the context is canceled or ListenAndServe fails.
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	logger.Infof("A2A Server starting at http://%s", addr)
	logger.Infof("Agent Card: http://%s/.well-known/agent-card.json", addr)

	if s.registrationConfig != nil && s.autoRegister {
		s.registerWithAIP(ctx)
	}
	if cfg := s.registrationConfig; cfg != nil && cfg.EndpointURL == "" && cfg.GatewayURL != "" {
		pollCtx, cancel := context.WithCancel(ctx)
		s.pollCancel = cancel
		go s.gatewayPollingLoop(pollCtx)
	}

	httpServer := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		if s.pollCancel != nil {
			s.pollCancel()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		logger.Infof("A2A Server shutting down")
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) serializeAgentCard() map[string]any {
	b, _ := json.Marshal(s.AgentCard)
	var card map[string]any
	_ = json.Unmarshal(b, &card)
	if cfg := s.registrationConfig; cfg != nil && cfg.EndpointURL != "" {
		card["endpoint_url"] = cfg.EndpointURL
		card["url"] = cfg.EndpointURL
	}
	if _, ok := card["url"]; !ok {
		card["url"] = fmt.Sprintf("http://%s:%d", s.host, s.port)
	}
	return card
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.serializeAgentCard())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "agent": s.AgentCard.Name})
}

type jsonRPCBody struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

func rpcError(id any, code int, message string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
}

func rpcResult(id any, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	var body jsonRPCBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcError(nil, a2a.ErrParseError, "Invalid JSON"))
		return
	}
	if body.JSONRPC != "2.0" || body.Method == "" {
		writeJSON(w, http.StatusBadRequest, rpcError(body.ID, a2a.ErrInvalidRequest, "Invalid JSON-RPC request"))
		return
	}
	var params map[string]any
	if len(body.Params) > 0 {
		_ = json.Unmarshal(body.Params, &params)
	}
	if params == nil {
		params = map[string]any{}
	}

	// message/stream switches the same endpoint to SSE so single-URL JSON-RPC
	// clients (e.g. a2a-go's transport) can stream against / and /a2a.
	if body.Method == "message/stream" {
		s.streamRPC(r.Context(), w, body.ID, params)
		return
	}

	var result any
	var err error
	switch body.Method {
	case "message/send":
		result, err = s.handleMessageSend(r.Context(), params)
	case "tasks/get":
		result, err = s.handleTasksGet(params)
	case "tasks/list":
		result, err = s.handleTasksList(params)
	case "tasks/cancel":
		result, err = s.handleTasksCancel(params)
	default:
		writeJSON(w, http.StatusOK, rpcError(body.ID, a2a.ErrMethodNotFound, "Method not found: "+body.Method))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, rpcError(body.ID, a2a.ErrTaskNotFound, err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, rpcResult(body.ID, result))
}

func parseMessage(data any) (*a2a.Message, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var msg a2a.Message
	if err := json.Unmarshal(b, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func strParam(params map[string]any, key string) string {
	s, _ := params[key].(string)
	return s
}

// taskIDs resolves the task and context IDs from the request params, falling
// back to the message's own taskId/contextId (which is where A2A clients such
// as a2a-go carry them in MessageSendParams). A missing task ID is generated.
func taskIDs(params map[string]any, message *a2a.Message) (taskID, contextID string) {
	taskID = strParam(params, "id")
	if taskID == "" {
		taskID = string(message.TaskID)
	}
	if taskID == "" {
		taskID = uuid.NewString()
	}
	contextID = strParam(params, "contextId")
	if contextID == "" {
		contextID = message.ContextID
	}
	return taskID, contextID
}

func isTerminal(state a2a.TaskState) bool {
	switch state {
	case a2a.TaskStateCompleted, a2a.TaskStateFailed, a2a.TaskStateCanceled:
		return true
	}
	return false
}

// runHandler drives the task handler, accumulating history/artifacts and
// finalizing task state. It returns the updated task.
func (s *Server) runHandler(ctx context.Context, task *a2a.Task, message *a2a.Message) *a2a.Task {
	history := append([]*a2a.Message{}, task.History...)
	artifacts := append([]*a2a.Artifact{}, task.Artifacts...)

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("Error in task handler: %v", rec)
				task.Status = a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), fmt.Sprintf("Error: %v", rec))}
			}
		}()
		for resp := range s.handler(ctx, task, message) {
			if resp.Message != nil {
				history = append(history, resp.Message)
			}
			if resp.StatusUpdate != nil {
				task.Status = resp.StatusUpdate.Status
			}
			if resp.ArtifactUpdate != nil && resp.ArtifactUpdate.Artifact != nil {
				artifacts = append(artifacts, resp.ArtifactUpdate.Artifact)
			}
		}
	}()

	finalState := task.Status.State
	if !isTerminal(finalState) {
		finalState = a2a.TaskStateCompleted
	}
	task.History = history
	task.Artifacts = artifacts
	if task.Status.State != a2a.TaskStateFailed {
		task.Status = a2a.TaskStatus{State: finalState}
	}
	return task
}

func (s *Server) getOrCreateTask(taskID, contextID string, message *a2a.Message) *a2a.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.tasks[taskID]; ok {
		existing.History = append(existing.History, message)
		return existing
	}
	if contextID == "" {
		contextID = uuid.NewString()
	}
	task := &a2a.Task{
		ID:        a2a.TaskID(taskID),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateSubmitted},
		History:   []*a2a.Message{message},
	}
	s.tasks[taskID] = task
	return task
}

func (s *Server) handleMessageSend(ctx context.Context, params map[string]any) (map[string]any, error) {
	message, err := parseMessage(params["message"])
	if err != nil {
		return nil, err
	}
	taskID, contextID := taskIDs(params, message)
	task := s.getOrCreateTask(taskID, contextID, message)

	s.mu.Lock()
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.Metadata["last_updated"] = time.Now().UTC().Format(time.RFC3339)
	task.Status = a2a.TaskStatus{State: a2a.TaskStateWorking}
	s.mu.Unlock()

	task = s.runHandler(ctx, task, message)

	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()

	return s.serializeTask(task), nil
}

func (s *Server) serializeTask(task *a2a.Task) map[string]any {
	b, _ := json.Marshal(task)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Server) handleTasksGet(params map[string]any) (map[string]any, error) {
	taskID := strParam(params, "id")
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	s.mu.Unlock()
	if taskID == "" || !ok {
		return nil, fmt.Errorf("Task not found: %s", taskID)
	}
	return s.serializeTask(task), nil
}

func (s *Server) handleTasksList(params map[string]any) (map[string]any, error) {
	contextID := strParam(params, "contextId")
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := make([]map[string]any, 0, len(s.tasks))
	for _, t := range s.tasks {
		if contextID != "" && t.ContextID != contextID {
			continue
		}
		tasks = append(tasks, s.serializeTask(t))
	}
	return map[string]any{"tasks": tasks}, nil
}

func (s *Server) handleTasksCancel(params map[string]any) (map[string]any, error) {
	taskID := strParam(params, "id")
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if taskID == "" || !ok {
		return nil, fmt.Errorf("Task not found: %s", taskID)
	}
	if isTerminal(task.Status.State) {
		return nil, fmt.Errorf("Task %s is already in terminal state", taskID)
	}
	task.Status = a2a.TaskStatus{State: a2a.TaskStateCanceled}
	return s.serializeTask(task), nil
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	var body jsonRPCBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcError(nil, a2a.ErrParseError, "Invalid JSON"))
		return
	}
	if body.Method != "message/stream" {
		writeJSON(w, http.StatusBadRequest, rpcError(body.ID, a2a.ErrMethodNotFound, "Streaming only supports message/stream"))
		return
	}
	var params map[string]any
	_ = json.Unmarshal(body.Params, &params)
	s.streamRPC(r.Context(), w, body.ID, params)
}

// streamRPC handles a message/stream request, emitting SSE events. It is shared
// by the /a2a/stream endpoint and the JSON-RPC endpoint (so a single-URL client
// like a2a-go's JSON-RPC transport works too).
func (s *Server) streamRPC(ctx context.Context, w http.ResponseWriter, id any, params map[string]any) {
	message, err := parseMessage(params["message"])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, rpcError(id, a2a.ErrInternalError, err.Error()))
		return
	}
	taskID, contextID := taskIDs(params, message)
	task := s.getOrCreateTask(taskID, contextID, message)
	task.Status = a2a.TaskStatus{State: a2a.TaskStateWorking}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, rpcError(id, a2a.ErrInternalError, "streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	history := append([]*a2a.Message{}, task.History...)
	artifacts := append([]*a2a.Artifact{}, task.Artifacts...)
	emit := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	for resp := range s.handler(ctx, task, message) {
		if ev := resp.Event(); ev != nil {
			emit(rpcResult(id, ev))
		}
		if resp.Message != nil {
			history = append(history, resp.Message)
		}
		if resp.StatusUpdate != nil {
			task.Status = resp.StatusUpdate.Status
		}
		if resp.ArtifactUpdate != nil && resp.ArtifactUpdate.Artifact != nil {
			artifacts = append(artifacts, resp.ArtifactUpdate.Artifact)
		}
	}

	finalState := task.Status.State
	if !isTerminal(finalState) {
		finalState = a2a.TaskStateCompleted
	}
	task.History = history
	task.Artifacts = artifacts
	task.Status = a2a.TaskStatus{State: finalState}
	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()
	emit(rpcResult(id, s.serializeTask(task)))
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	var req a2a.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "success": false})
		return
	}
	runID := ""
	if req.Context != nil {
		runID, _ = req.Context["run_id"].(string)
	}
	if runID == "" {
		runID = uuid.NewString()
	}
	agentID := s.AgentID()
	if agentID == "" && s.registrationConfig != nil {
		agentID = "erc8004:" + s.registrationConfig.Handle
	}

	msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), req.Message)
	msg.Metadata = req.Context
	params := map[string]any{"id": runID, "message": msg}
	taskResult, err := s.handleMessageSend(r.Context(), params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "success": false})
		return
	}

	content := extractAgentText(taskResult)
	state := ""
	if status, ok := taskResult["status"].(map[string]any); ok {
		state, _ = status["state"].(string)
	}
	resp := a2a.InvokeResponse{
		RunID:   runID,
		AgentID: agentID,
		Success: state == string(a2a.TaskStateCompleted),
		Content: content,
	}
	if meta, ok := taskResult["metadata"].(map[string]any); ok {
		resp.Data = meta
	}
	writeJSON(w, http.StatusOK, resp)
}

func extractAgentText(task map[string]any) string {
	history, _ := task["history"].([]any)
	for i := len(history) - 1; i >= 0; i-- {
		msg, _ := history[i].(map[string]any)
		if msg == nil {
			continue
		}
		if role, _ := msg["role"].(string); role == string(a2a.RoleAgent) {
			parts, _ := msg["parts"].([]any)
			if len(parts) > 0 {
				if p, ok := parts[0].(map[string]any); ok {
					if t, ok := p["text"].(string); ok {
						return t
					}
				}
			}
		}
	}
	return ""
}

func (s *Server) handleInvokeStream(w http.ResponseWriter, r *http.Request) {
	var req a2a.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "success": false})
		return
	}
	runID := ""
	if req.Context != nil {
		runID, _ = req.Context["run_id"].(string)
	}
	if runID == "" {
		runID = uuid.NewString()
	}
	msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), req.Message)
	msg.Metadata = req.Context
	task := s.getOrCreateTask(runID, "", msg)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported", "success": false})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	for resp := range s.handler(r.Context(), task, msg) {
		if resp.RawContent != "" {
			fmt.Fprint(w, resp.RawContent)
			flusher.Flush()
			continue
		}
		if ev := resp.Event(); ev != nil {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 20)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)

	s.mu.Lock()
	var withTime []*a2a.Task
	for _, t := range s.tasks {
		if t.Metadata != nil {
			if _, ok := t.Metadata["last_updated"]; ok {
				withTime = append(withTime, t)
			}
		}
	}
	s.mu.Unlock()

	sort.Slice(withTime, func(i, j int) bool {
		ti, _ := withTime[i].Metadata["last_updated"].(string)
		tj, _ := withTime[j].Metadata["last_updated"].(string)
		return ti > tj
	})

	total := len(withTime)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	data := make([]map[string]any, 0, end-start)
	for _, t := range withTime[start:end] {
		data = append(data, s.serializeTask(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("conversation_id")
	s.mu.Lock()
	task, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Conversation not found"})
		return
	}
	writeJSON(w, http.StatusOK, s.serializeTask(task))
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
