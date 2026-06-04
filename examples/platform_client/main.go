// Command platform_client calls agents through the AIP platform, mirroring the
// Python examples/client_example.py: direct calls by handle, auto-routing, and
// real-time event streaming.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/unibaseio/unibase-aip-sdk-go/platform"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	aipEndpoint := getenv("AIP_ENDPOINT", "http://localhost:8001")
	userID := "user:0x5eA13664c5ce67753f208540d25B913788Aa3DaA"

	client := platform.New(aipEndpoint)
	ctx := context.Background()

	fmt.Println("AIP Client Examples")
	fmt.Printf("Endpoint: %s\n\n", aipEndpoint)

	if !client.HealthCheck(ctx) {
		fmt.Println("ERROR: AIP platform is not available at", aipEndpoint)
		os.Exit(1)
	}
	fmt.Println("Platform is healthy")

	callWeatherAgent(ctx, client, userID)
	callCalculatorAgent(ctx, client, userID)
	streamAgentEvents(ctx, client, userID)
	autoRouteRequest(ctx, client, userID)
}

// Example 1: call a public agent by handle.
func callWeatherAgent(ctx context.Context, client *platform.Client, userID string) {
	fmt.Println("\n=== Example 1: Public Weather Agent ===")
	result, err := client.Run(ctx, "What's the weather in Tokyo?", platform.RunOptions{
		Agent:   "weather_public",
		UserID:  userID,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Success: %v\nStatus: %s\nOutput: %v\n", result.Success(), result.Status, result.Output())
}

// Example 2: call a private agent by handle.
func callCalculatorAgent(ctx context.Context, client *platform.Client, userID string) {
	fmt.Println("\n=== Example 2: Private Calculator Agent ===")
	result, err := client.Run(ctx, "Calculate 25 * 4 + 10", platform.RunOptions{
		Agent:   "calculator_private",
		UserID:  userID,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Success: %v\nOutput: %v\n", result.Success(), result.Output())
}

// Example 3: stream events in real time.
func streamAgentEvents(ctx context.Context, client *platform.Client, userID string) {
	fmt.Println("\n=== Example 3: Streaming Events ===")
	for ev := range client.RunStream(ctx, "Calculate 50 * 2", platform.RunOptions{
		Agent:  "calculator_private",
		UserID: userID,
	}) {
		if ev.Err != nil {
			fmt.Println("stream error:", ev.Err)
			return
		}
		ts := time.Now().Format("15:04:05")
		fmt.Printf("[%s] %s\n", ts, ev.Event.EventType)
		switch ev.Event.EventType {
		case "agent_invoked":
			fmt.Printf("  -> Agent started: %v\n", ev.Event.Payload["agent"])
		case "payment.settled":
			fmt.Printf("  -> Payment: $%v USD\n", ev.Event.Payload["amount"])
		case "memory_uploaded":
			fmt.Printf("  -> Memory: %v\n", ev.Event.Payload["operation"])
		case "agent_completed":
			fmt.Println("  -> Agent completed")
		}
		if ev.Event.IsCompleted() {
			fmt.Printf("  -> Final output: %v\n", ev.Event.Payload["output"])
			break
		}
	}
}

// Example 4: let the platform auto-select the best agent (no Agent set).
func autoRouteRequest(ctx context.Context, client *platform.Client, userID string) {
	fmt.Println("\n=== Example 4: Auto-routing ===")
	for _, objective := range []string{"What is 144 divided by 12?", "How's the weather in Paris?"} {
		result, err := client.Run(ctx, objective, platform.RunOptions{UserID: userID, Timeout: 30 * time.Second})
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("Q: %s\n  -> %v\n", objective, result.Output())
	}
}
