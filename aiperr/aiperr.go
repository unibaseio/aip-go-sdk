// Package aiperr defines the error types used across the Unibase AIP SDK,
// mirroring aip_sdk/exceptions.py and aip_sdk/core/exceptions.py.
package aiperr

import "fmt"

// Error is the base error for all AIP SDK errors. It carries an optional
// machine-readable code and a free-form details map.
type Error struct {
	Message string
	Code    string
	Details map[string]any
}

// New creates an Error with the given message and optional code/details.
func New(message string, code string, details map[string]any) *Error {
	if details == nil {
		details = map[string]any{}
	}
	return &Error{Message: message, Code: code, Details: details}
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return e.Message
}

// set stores a detail value, allocating the map if needed.
func (e *Error) set(key string, value any) {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
}

// Error codes, matching the Python SDK's string codes.
const (
	CodeConnection         = "CONNECTION_ERROR"
	CodeAuth               = "AUTH_ERROR"
	CodeRegistration       = "REGISTRATION_ERROR"
	CodeExecution          = "EXECUTION_ERROR"
	CodePayment            = "PAYMENT_ERROR"
	CodeValidation         = "VALIDATION_ERROR"
	CodeTimeout            = "TIMEOUT_ERROR"
	CodeAgentNotFound      = "AGENT_NOT_FOUND"
	CodeStorage            = "STORAGE_ERROR"
	CodeInit               = "INIT_ERROR"
	CodeConfig             = "CONFIG_ERROR"
	CodeRegistry           = "REGISTRY_ERROR"
	CodeMemory             = "MEMORY_ERROR"
	CodeMiddleware         = "MIDDLEWARE_ERROR"
	CodeMiddlewareNotAvail = "MIDDLEWARE_NOT_AVAILABLE"
	CodeA2A                = "A2A_ERROR"
	CodeDiscovery          = "DISCOVERY_ERROR"
	CodeTaskExec           = "TASK_EXEC_ERROR"
	CodeWallet             = "WALLET_ERROR"
)

// Connection returns an error connecting to the AIP platform.
func Connection(message string, url string) *Error {
	if message == "" {
		message = "Failed to connect to AIP platform"
	}
	e := New(message, CodeConnection, nil)
	if url != "" {
		e.set("url", url)
	}
	return e
}

// Authentication returns an authentication or authorization error.
func Authentication(message string) *Error {
	if message == "" {
		message = "Authentication failed"
	}
	return New(message, CodeAuth, nil)
}

// Registration returns an error during agent registration.
func Registration(message, agentID, handle string) *Error {
	if message == "" {
		message = "Agent registration failed"
	}
	e := New(message, CodeRegistration, nil)
	if agentID != "" {
		e.set("agent_id", agentID)
	}
	if handle != "" {
		e.set("handle", handle)
	}
	return e
}

// Execution returns an error during task execution.
func Execution(message, taskID, agentID, runID string) *Error {
	if message == "" {
		message = "Task execution failed"
	}
	e := New(message, CodeExecution, nil)
	if taskID != "" {
		e.set("task_id", taskID)
	}
	if agentID != "" {
		e.set("agent_id", agentID)
	}
	if runID != "" {
		e.set("run_id", runID)
	}
	return e
}

// Payment returns an error related to payment processing.
func Payment(message string) *Error {
	if message == "" {
		message = "Payment processing failed"
	}
	return New(message, CodePayment, nil)
}

// Validation returns an error validating input data.
func Validation(message, field string) *Error {
	if message == "" {
		message = "Validation failed"
	}
	e := New(message, CodeValidation, nil)
	if field != "" {
		e.set("field", field)
	}
	return e
}

// Timeout returns an operation-timed-out error.
func Timeout(message string, timeoutSeconds float64) *Error {
	if message == "" {
		message = "Operation timed out"
	}
	e := New(message, CodeTimeout, nil)
	if timeoutSeconds > 0 {
		e.set("timeout_seconds", timeoutSeconds)
	}
	return e
}

// AgentNotFound returns an error when the requested agent was not found.
func AgentNotFound(message, agentID string) *Error {
	if message == "" {
		message = "Agent not found"
	}
	e := New(message, CodeAgentNotFound, nil)
	if agentID != "" {
		e.set("agent_id", agentID)
	}
	return e
}

// Storage returns an error during storage operations.
func Storage(message, path, operation string) *Error {
	if message == "" {
		message = "Storage operation failed"
	}
	e := New(message, CodeStorage, nil)
	if path != "" {
		e.set("path", path)
	}
	if operation != "" {
		e.set("operation", operation)
	}
	return e
}

// Initialization returns a component-initialization error.
func Initialization(message string) *Error {
	if message == "" {
		message = "Initialization failed"
	}
	return New(message, CodeInit, nil)
}

// Configuration returns an invalid-configuration error.
func Configuration(message string) *Error {
	if message == "" {
		message = "Configuration error"
	}
	return New(message, CodeConfig, nil)
}

// Registry returns a registry-related error.
func Registry(message string) *Error {
	if message == "" {
		message = "Registry error"
	}
	return New(message, CodeRegistry, nil)
}

// Memory returns a memory-operation error.
func Memory(message string) *Error {
	if message == "" {
		message = "Memory operation failed"
	}
	return New(message, CodeMemory, nil)
}

// A2AProtocol returns an A2A protocol error.
func A2AProtocol(message string) *Error {
	if message == "" {
		message = "A2A protocol error"
	}
	return New(message, CodeA2A, nil)
}

// AgentDiscovery returns an agent-discovery error.
func AgentDiscovery(message string) *Error {
	if message == "" {
		message = "Agent discovery failed"
	}
	return New(message, CodeDiscovery, nil)
}

// TaskExecution returns an A2A task-execution error.
func TaskExecution(message string) *Error {
	if message == "" {
		message = "Task execution failed"
	}
	return New(message, CodeTaskExec, nil)
}

// Wallet returns a Web3 wallet-related error.
func Wallet(message string) *Error {
	if message == "" {
		message = "Wallet error"
	}
	return New(message, CodeWallet, nil)
}
