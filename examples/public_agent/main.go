// Command public_agent runs a publicly reachable weather agent in Gateway
// DIRECT mode, mirroring the Python examples/public_agent_full.py. The agent is
// registered with the platform and exposes a public endpoint the Gateway calls.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/unibaseio/unibase-aip-sdk-go/types"
	"github.com/unibaseio/unibase-aip-sdk-go/wrappers"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// WeatherAgent provides mock weather information.
type WeatherAgent struct{}

func (w *WeatherAgent) Handle(ctx context.Context, input string) (string, error) {
	fmt.Printf("[Weather Agent] request: %s\n", input)

	city := "Tokyo"
	for _, c := range []string{"tokyo", "paris", "london", "new york", "beijing", "shanghai"} {
		if strings.Contains(strings.ToLower(input), c) {
			city = titleCase(c)
			break
		}
	}

	conditions := []string{"Sunny", "Cloudy", "Rainy", "Partly Cloudy"}
	temp := rand.Intn(16) + 15
	cond := conditions[rand.Intn(len(conditions))]
	humidity := rand.Intn(41) + 40
	wind := rand.Intn(16) + 5

	var b strings.Builder
	fmt.Fprintf(&b, "Weather in %s\n", city)
	fmt.Fprintf(&b, "Temperature: %d C\n", temp)
	fmt.Fprintf(&b, "Condition: %s\n", cond)
	fmt.Fprintf(&b, "Humidity: %d%%\n", humidity)
	fmt.Fprintf(&b, "Wind Speed: %d km/h\n", wind)
	return b.String(), nil
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func main() {
	userWallet := getenv("MEMBASE_ACCOUNT", "0x5ea13664c5ce67753f208540d25b913788aa3daa")
	aipEndpoint := getenv("AIP_ENDPOINT", "http://localhost:8001")
	gatewayURL := getenv("GATEWAY_URL", "http://localhost:8080")
	host := getenv("AGENT_HOST", "0.0.0.0")
	port, _ := strconv.Atoi(getenv("AGENT_PORT", "8200"))
	endpointURL := getenv("AGENT_PUBLIC_URL", fmt.Sprintf("http://your-public-ip:%d", port))

	base := 0.001
	costModel := &types.CostModel{BaseCallFee: &base}

	skills := []types.AgentSkillCard{{
		ID:          "weather.query",
		Name:        "Weather Query",
		Description: "Get current weather information for any city",
		Tags:        []string{"weather", "forecast", "temperature"},
		Examples:    []string{"What's the weather in Tokyo?", "Forecast for Paris"},
	}}

	agent := &WeatherAgent{}

	// DIRECT mode: a non-empty EndpointURL means the Gateway calls the agent
	// directly (no polling). AutoRegister is enabled so the agent registers on
	// start; set DisableAutoRegister to register out of band instead.
	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:        "Public Weather Agent",
		Description: "A public weather agent providing real-time weather information",
		Host:        host,
		Port:        port,
		Skills:      skills,
		UserID:      "user:" + userWallet,
		AIPEndpoint: aipEndpoint,
		GatewayURL:  gatewayURL,
		Handle:      "weather_public",
		CostModel:   costModel,
		EndpointURL: endpointURL, // DIRECT mode
		Metadata:    map[string]any{"version": "1.0.0", "mode": "public", "deployment": "direct"},
	}, agent.Handle, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Public Weather Agent on http://%s:%d (DIRECT mode)\n", host, port)
	fmt.Printf("Agent Card: http://%s:%d/.well-known/agent-card.json\n", host, port)
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
