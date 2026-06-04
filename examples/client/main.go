// Command client sends a task to a locally running A2A agent (see the server
// example) and prints the agent's reply.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/unibaseio/unibase-aip-sdk-go/a2a"
)

func main() {
	agentURL := "http://127.0.0.1:8000"
	if len(os.Args) > 1 {
		agentURL = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := a2a.NewClient(0, nil)

	card, err := client.DiscoverAgent(ctx, agentURL, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover error:", err)
		os.Exit(1)
	}
	fmt.Printf("Discovered agent: %s — %s\n", card.Name, card.Description)

	msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), "hello from the Go client")
	task, err := client.SendTask(ctx, agentURL, msg, "", "", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "send error:", err)
		os.Exit(1)
	}

	fmt.Printf("Task %s state: %s\n", task.ID, task.Status.State)
	for _, m := range task.History {
		if m.Role == a2a.RoleAgent {
			fmt.Println("Agent:", a2a.GetMessageText(m))
		}
	}
}
