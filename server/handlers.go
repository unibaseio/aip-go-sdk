package server

import (
	"context"

	"github.com/google/uuid"

	"github.com/unibaseio/aip-go-sdk/a2a"
)

// TextHandler maps input text to a response string.
type TextHandler func(ctx context.Context, input string) (string, error)

// StreamTextHandler maps input text to a stream of response chunks delivered on
// the returned channel, which must be closed when complete.
type StreamTextHandler func(ctx context.Context, input string) <-chan string

// CreateSimpleHandler adapts a text-to-text function into a TaskHandler.
func CreateSimpleHandler(fn TextHandler, rawResponse bool) TaskHandler {
	return func(ctx context.Context, task *a2a.Task, message *a2a.Message) <-chan a2a.StreamResponse {
		ch := make(chan a2a.StreamResponse, 1)
		go func() {
			defer close(ch)
			input := a2a.GetMessageText(message)
			result, err := fn(ctx, input)
			if err != nil {
				ch <- a2a.StreamResponse{StatusUpdate: &a2a.TaskStatusUpdateEvent{
					TaskID:    task.ID,
					ContextID: task.ContextID,
					Status:    a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), "Error: "+err.Error())},
					Final:     true,
				}}
				return
			}
			if rawResponse {
				ch <- a2a.StreamResponse{RawContent: result}
			} else {
				ch <- a2a.StreamResponse{Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), result)}
			}
		}()
		return ch
	}
}

// CreateStreamHandler adapts a streaming text function into a TaskHandler.
func CreateStreamHandler(fn StreamTextHandler, rawResponse bool) TaskHandler {
	return func(ctx context.Context, task *a2a.Task, message *a2a.Message) <-chan a2a.StreamResponse {
		ch := make(chan a2a.StreamResponse)
		go func() {
			defer close(ch)
			input := a2a.GetMessageText(message)
			for chunk := range fn(ctx, input) {
				if rawResponse {
					ch <- a2a.StreamResponse{RawContent: chunk}
				} else {
					ch <- a2a.StreamResponse{Message: a2a.NewMessage(a2a.RoleAgent, uuid.NewString(), chunk)}
				}
			}
		}()
		return ch
	}
}
