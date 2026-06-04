package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/unibaseio/unibase-aip-sdk-go/a2a"
	"github.com/unibaseio/unibase-aip-sdk-go/types"
)

func newTestServer() *Server {
	card := types.AgentCard{Name: "Test", Description: "t", Version: "1.0.0"}
	handler := CreateSimpleHandler(func(ctx context.Context, input string) (string, error) {
		return "reply: " + input, nil
	}, false)
	return New(card, handler, "127.0.0.1", 8000)
}

func TestAgentCardEndpoint(t *testing.T) {
	ts := httptest.NewServer(newTestServer().Handler())
	defer ts.Close()

	client := a2a.NewClient(0, nil)
	card, err := client.DiscoverAgent(context.Background(), ts.URL, false)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if card.Name != "Test" {
		t.Fatalf("name = %q", card.Name)
	}
}

func TestMessageSendEndToEnd(t *testing.T) {
	ts := httptest.NewServer(newTestServer().Handler())
	defer ts.Close()

	client := a2a.NewClient(0, nil)
	msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), "ping")
	task, err := client.SendTask(context.Background(), ts.URL, msg, "task-1", "", nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %q, want completed", task.Status.State)
	}
	var got string
	for _, m := range task.History {
		if m.Role == a2a.RoleAgent {
			got = a2a.GetMessageText(m)
		}
	}
	if !strings.Contains(got, "reply: ping") {
		t.Fatalf("agent reply = %q", got)
	}
}

func TestTasksGetAndCancel(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := a2a.NewClient(0, nil)
	msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), "ping")
	if _, err := client.SendTask(context.Background(), ts.URL, msg, "task-2", "", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := client.GetTask(context.Background(), ts.URL, "task-2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "task-2" {
		t.Fatalf("id = %q", got.ID)
	}
}
