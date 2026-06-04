// Command server exposes a simple echo/calculator agent over the A2A protocol.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/unibaseio/aip-go-sdk/server"
	"github.com/unibaseio/aip-go-sdk/wrappers"
)

func main() {
	handler := func(ctx context.Context, input string) (string, error) {
		input = strings.TrimSpace(input)
		if input == "" {
			return "Please say something.", nil
		}
		return fmt.Sprintf("Echo: %s", input), nil
	}

	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:        "Echo Agent",
		Description: "Echoes back whatever you send it",
		Host:        "127.0.0.1",
		Port:        8000,
		// No UserID set, so the agent runs without platform registration.
	}, server.TextHandler(handler), nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("Serving on http://127.0.0.1:8000 (Ctrl+C to stop)")
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
