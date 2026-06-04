// Command prediction_market_agent is a minimal end-to-end marketplace agent,
// mirroring the Python examples/prediction_market_agent.py:
//
//  1. Auto-registration on the AIP marketplace (auto-register enabled).
//  2. POLLING + job-queue mode (ViaGateway=true, no public endpoint).
//  3. A handler that returns ONE JSON deliverable matching the offering's
//     deliverable schema ({"text": "..."}).
//  4. A single AgentJobOffering with input/deliverable schemas and fixed-price
//     USDC pricing.
//  5. JWT-based auth: load UNIBASE_PROXY_AUTH (env or ~/.unibase/aip-config.json),
//     or run an interactive first-time flow; the wallet is the JWT `sub` claim.
//
// The Python version calls OpenAI for the estimate; to stay dependency-free
// this computes a deterministic offline mock in the same output format.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

func configFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".unibase", "aip-config.json")
}

// loadAuthToken reads the JWT from UNIBASE_PROXY_AUTH, then the config file.
func loadAuthToken() string {
	if env := os.Getenv("UNIBASE_PROXY_AUTH"); env != "" {
		return env
	}
	data, err := os.ReadFile(configFile())
	if err != nil {
		return ""
	}
	var cfg map[string]string
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg["UNIBASE_PROXY_AUTH"]
}

func saveAuthToken(token string) error {
	path := configFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(map[string]string{"UNIBASE_PROXY_AUTH": token}, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Printf("  saved token to %s\n", path)
	return nil
}

// interactiveAuth fetches an authorization URL and reads the signed JWT from stdin.
func interactiveAuth(ctx context.Context) (string, error) {
	payURL := getenv("UNIBASE_PAY_URL", "https://api.pay.unibase.com")
	fmt.Println("\n=== Step 1: Interactive Authorization (first run only) ===")
	fmt.Println("[1/3] Fetching authorization URL ...")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payURL+"/v1/init", strings.NewReader("true"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	authURL, _ := parsed["auth_url"].(string)
	if authURL == "" {
		authURL, _ = parsed["authUrl"].(string)
	}
	if authURL == "" {
		return "", fmt.Errorf("no auth URL in response: %s", body)
	}
	fmt.Printf("\n[2/3] Open this URL in your browser and sign with your wallet:\n\n  %s\n\n", authURL)
	fmt.Println("[3/3] Paste the JWT token returned after signing, then press Enter:")
	fmt.Print("  Token: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return "", fmt.Errorf("no token provided — aborted")
	}
	_ = saveAuthToken(token)
	return token, nil
}

func ensureAuth(ctx context.Context) (string, error) {
	if token := loadAuthToken(); token != "" {
		fmt.Println("loaded cached UNIBASE_PROXY_AUTH")
		return token, nil
	}
	return interactiveAuth(ctx)
}

// extractWalletFromToken decodes the JWT payload and returns its `sub` claim.
func extractWalletFromToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Fall back to standard base64 with padding.
		p := parts[1]
		if pad := len(p) % 4; pad != 0 {
			p += strings.Repeat("=", 4-pad)
		}
		payload, err = base64.StdEncoding.DecodeString(p)
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

// handler returns the YES/NO analysis as a JSON string matching the deliverable
// schema ({"text": "..."}). The estimate is a deterministic offline mock.
func handler(ctx context.Context, input string) (string, error) {
	fmt.Printf("[handler] input=%q\n", input)

	prompt := input
	if strings.HasPrefix(strings.TrimSpace(input), "{") {
		var data map[string]any
		if json.Unmarshal([]byte(input), &data) == nil {
			for _, key := range []string{"topic", "user_request", "text", "content"} {
				if v, ok := data[key].(string); ok && v != "" {
					prompt = v
					break
				}
			}
		}
	}

	yes := mockProbability(prompt)
	text := fmt.Sprintf("Topic: %s\nYES: %d%%\nNO: %d%%\nReasoning: Estimate from base rates and priors for the stated question.",
		asQuestion(prompt), yes, 100-yes)
	fmt.Printf("[handler] output=%q\n", text)

	// Return the deliverable text directly (not wrapped in a {"text": ...} object).
	return text, nil
}

func mockProbability(topic string) int {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(topic))))
	return int(sum[0]) % 101 // 0..100
}

func asQuestion(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "Will the event occur?"
	}
	if !strings.HasSuffix(topic, "?") {
		return topic + "?"
	}
	return topic
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	authToken, err := ensureAuth(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth error:", err)
		os.Exit(1)
	}
	userID := extractWalletFromToken(authToken)
	if userID == "" {
		fmt.Println("ERROR: token did not contain a usable wallet (sub claim missing).")
		fmt.Println("  -> Delete ~/.unibase/aip-config.json and re-run to re-authorize.")
		os.Exit(1)
	}

	jobOfferings := []types.AgentJobOffering{{
		ID:          "yes_no_probability",
		Name:        "yes_no_probability",
		Description: "Estimates YES/NO probabilities for any prediction market topic. Returns a fixed format: restated question, integer YES%, integer NO% (sums to 100), and a one-sentence rationale.",
		Type:        "JOB",
		Price:       0.0,
		PriceV2:     map[string]any{"type": "fixed", "amount": 0.0015, "currency": "USDC"},
		JobInput:    "Will BTC break $150k by end of 2026?",
		JobOutput:   "Topic: <restated question>\nYES: <0-100>%\nNO: <0-100>%\nReasoning: <short rationale>",
		Requirement: map[string]any{
			"type":     "object",
			"required": []string{"topic"},
			"properties": map[string]any{
				"topic": map[string]any{"type": "string", "description": "A yes/no question or topic to estimate."},
			},
		},
		Deliverable: map[string]any{
			"type":     "object",
			"required": []string{"text"},
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "Full YES/NO analysis in the format above."},
			},
		},
		SLAMinutes:    1,
		RequiredFunds: false,
		Restricted:    false,
		Active:        true,
	}}

	base := 0.0015
	skills := []types.AgentSkillCard{{
		ID:          "prediction.yes_no",
		Name:        "YES/NO Probability",
		Description: "Estimate the YES/NO probability of any topic.",
		Tags:        []string{"prediction", "probability", "analysis"},
		Examples:    []string{"Will BTC break $150k by end of 2026?", "Will OpenAI IPO before 2028?"},
	}}

	port, _ := strconv.Atoi(getenv("AGENT_PORT", "8201"))

	fmt.Println("Starting Prediction Market Agent on BSC Testnet (chain_id=97) ...")
	// POLLING + job-queue mode: ViaGateway=true and an empty EndpointURL make the
	// server poll the Gateway's job queue; auto-registration is enabled.
	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:         "Prediction Market Agent",
		Handle:       "prediction_market_demo",
		Description:  "AI agent that estimates YES/NO probabilities for any prediction market topic.",
		Host:         "0.0.0.0",
		Port:         port,
		Skills:       skills,
		UserID:       userID,
		PrivyToken:   authToken,
		AIPEndpoint:  getenv("AIP_ENDPOINT", "https://api.aip.unibase.com"),
		GatewayURL:   getenv("GATEWAY_URL", "https://gateway.aip.unibase.com"),
		EndpointURL:  "", // POLLING mode
		ViaGateway:   true,
		ChainID:      97,
		JobOfferings: jobOfferings,
		CostModel:    &types.CostModel{BaseCallFee: &base},
	}, handler, nil)

	fmt.Println("Polling Gateway for jobs. Ctrl+C to stop.")
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
