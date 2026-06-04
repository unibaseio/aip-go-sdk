// Command streaming_agent exposes a streaming agent, mirroring the Python
// examples/streaming_agent.py. The Python version streams tokens from OpenAI;
// to stay dependency-free this emits a mock token stream instead.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/unibaseio/aip-go-sdk/server"
	"github.com/unibaseio/aip-go-sdk/types"
	"github.com/unibaseio/aip-go-sdk/wrappers"
)

// streamingHandler emits the response one word at a time over a channel,
// simulating an LLM token stream.
func streamingHandler(ctx context.Context, input string) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		story := fmt.Sprintf("Once upon a time, an agent received the prompt %q and told a long story about it.", input)
		for _, word := range strings.Fields(story) {
			select {
			case <-ctx.Done():
				return
			case out <- word + " ":
				time.Sleep(30 * time.Millisecond)
			}
		}
	}()
	return out
}

func main() {
	skills := []types.AgentSkillCard{{
		ID:          "story.teller",
		Name:        "Story Teller",
		Description: "Generates long stories with streaming response",
		Tags:        []string{"creative", "story", "streaming"},
	}}

	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:        "Streaming Agent",
		Description: "Agent with streaming response",
		Host:        "127.0.0.1",
		Port:        8000,
		Skills:      skills,
		Streaming:   true,
	}, nil, server.StreamTextHandler(streamingHandler))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("Streaming agent on http://127.0.0.1:8000 — POST to /a2a/stream (Ctrl+C to stop)")
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
