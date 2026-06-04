// Command binance_agent is the Agent SDK startup guide, mirroring the Python
// examples/agent_sdk_startup_guide.py. It exposes a real Binance price-query
// agent with ERC-8183 job offerings, and supports four startup modes:
//
//	auto            - auto-register + PUSH mode (default)
//	manual          - manual register + PUSH mode
//	polling         - auto-register + POLLING mode (private agent)
//	polling-manual  - manual register + POLLING mode
//
// Run: go run ./examples/binance_agent [mode]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/unibaseio/aip-go-sdk/examples/internal/exauth"
	"github.com/unibaseio/aip-go-sdk/platform"
	"github.com/unibaseio/aip-go-sdk/types"
	"github.com/unibaseio/aip-go-sdk/wrappers"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// BinancePriceAgent answers crypto price/stats/klines/orderbook queries using
// Binance's public API (no auth required).
type BinancePriceAgent struct {
	http *http.Client
}

const binanceBaseURL = "https://api.binance.com"

func (a *BinancePriceAgent) Handle(ctx context.Context, input string) (string, error) {
	fmt.Printf("[BinancePriceAgent] received: %s\n", input)
	lower := strings.ToLower(strings.TrimSpace(input))
	symbol := a.extractSymbol(strings.ToUpper(strings.TrimSpace(input)))
	if symbol == "" {
		return "Usage:\n  <SYMBOL> price\n  <SYMBOL> 24h change\n  <SYMBOL> klines <N>\n  <SYMBOL> orderbook <N>\n", nil
	}
	switch {
	case strings.Contains(lower, "kline") || strings.Contains(lower, "candle"):
		return a.getKlines(ctx, symbol, extractLimit(lower, 10)), nil
	case strings.Contains(lower, "24h") || strings.Contains(lower, "change") || strings.Contains(lower, "stats"):
		return a.get24hrStats(ctx, symbol), nil
	case strings.Contains(lower, "orderbook") || strings.Contains(lower, "depth"):
		return a.getOrderbook(ctx, symbol, extractLimit(lower, 5)), nil
	default:
		return a.getCurrentPrice(ctx, symbol), nil
	}
}

func (a *BinancePriceAgent) extractSymbol(text string) string {
	for _, w := range []string{"PRICE", "KLINE", "CANDLE", "24H", "CHANGE", "STATS", "ORDERBOOK", "DEPTH", "GET"} {
		text = strings.ReplaceAll(text, w, "")
	}
	for _, part := range strings.Fields(text) {
		part = strings.Trim(part, "!?.,")
		switch {
		case part == "":
			continue
		case strings.HasSuffix(part, "USDT"), strings.HasSuffix(part, "BTC"),
			strings.HasSuffix(part, "BNB"), strings.HasSuffix(part, "ETH"), strings.HasSuffix(part, "USD"):
			return part
		}
	}
	return "BTCUSDT"
}

var limitRe = regexp.MustCompile(`(\d+)\s*(kline|candle|limit|count|bar)`)
var lastRe = regexp.MustCompile(`last\s+(\d+)`)

func extractLimit(text string, def int) int {
	for _, re := range []*regexp.Regexp{limitRe, lastRe} {
		if m := re.FindStringSubmatch(text); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				if n > 1000 {
					n = 1000
				}
				return n
			}
		}
	}
	return def
}

func (a *BinancePriceAgent) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, binanceBaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Binance API error %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (a *BinancePriceAgent) getCurrentPrice(ctx context.Context, symbol string) string {
	var d struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := a.get(ctx, "/api/v3/ticker/price", url.Values{"symbol": {symbol}}, &d); err != nil {
		return "Failed to fetch price: " + err.Error()
	}
	return fmt.Sprintf("%s Current Price\n  Price:  $%s\n  Time:   %s",
		d.Symbol, d.Price, time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
}

func (a *BinancePriceAgent) get24hrStats(ctx context.Context, symbol string) string {
	var d map[string]any
	if err := a.get(ctx, "/api/v3/ticker/24hr", url.Values{"symbol": {symbol}}, &d); err != nil {
		return "Failed to fetch 24h stats: " + err.Error()
	}
	return fmt.Sprintf("%s 24h Stats\n  Change:     %v%%\n  Last Price: $%v\n  High:       $%v\n  Low:        $%v\n  Volume:     %v",
		symbol, d["priceChangePercent"], d["lastPrice"], d["highPrice"], d["lowPrice"], d["volume"])
}

func (a *BinancePriceAgent) getKlines(ctx context.Context, symbol string, limit int) string {
	var rows [][]any
	q := url.Values{"symbol": {symbol}, "interval": {"1d"}, "limit": {strconv.Itoa(limit)}}
	if err := a.get(ctx, "/api/v3/klines", q, &rows); err != nil {
		return "Failed to fetch klines: " + err.Error()
	}
	if len(rows) == 0 {
		return symbol + " No kline data available"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Daily Klines (last %d bars)\n", symbol, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		k := rows[i]
		openTime := time.UnixMilli(int64(k[0].(float64))).UTC().Format("2006-01-02")
		fmt.Fprintf(&b, "  %s  O:%v H:%v L:%v C:%v Vol:%v\n", openTime, k[1], k[2], k[3], k[4], k[5])
	}
	return b.String()
}

func (a *BinancePriceAgent) getOrderbook(ctx context.Context, symbol string, limit int) string {
	var d struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := a.get(ctx, "/api/v3/depth", url.Values{"symbol": {symbol}, "limit": {strconv.Itoa(limit)}}, &d); err != nil {
		return "Failed to fetch orderbook: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Orderbook (depth %d)\n  --- Bids ---\n", symbol, limit)
	for _, lv := range d.Bids {
		fmt.Fprintf(&b, "  $%s x %s\n", lv[0], lv[1])
	}
	b.WriteString("  --- Asks ---\n")
	for _, lv := range d.Asks {
		fmt.Fprintf(&b, "  $%s x %s\n", lv[0], lv[1])
	}
	return b.String()
}

// jobOfferings is the ERC-8183 marketplace listing for the Binance agent.
func jobOfferings() []types.AgentJobOffering {
	return []types.AgentJobOffering{{
		ID:          "binance_price_query",
		Name:        "Crypto Price Query",
		Description: "Query current price, 24h stats, klines, or orderbook for any crypto pair on Binance.",
		Type:        "JOB",
		Price:       0.0,
		PriceV2:     map[string]any{"type": "fixed", "amount": 0, "currency": "USDC"},
		JobInput:    "Text query: '<SYMBOL> price' | '<SYMBOL> 24h change' | '<SYMBOL> klines <N>' | '<SYMBOL> orderbook'",
		JobOutput:   "Text or JSON with price data, 24h stats, klines, or orderbook",
		Requirement: map[string]any{"type": "object", "required": []string{"ticker"}, "properties": map[string]any{"ticker": map[string]any{"type": "string", "description": "ticker"}}},
		Deliverable: map[string]any{"type": "object", "required": []string{"text"}, "properties": map[string]any{"text": map[string]any{"type": "string", "description": "Complete deliverable"}}},
		SLAMinutes:  1,
		Active:      true,
	}}
}

func jobResources() []types.AgentJobResource {
	return []types.AgentJobResource{{
		ID:          "binance_api",
		URL:         "https://api.binance.com",
		Name:        "Binance Public API",
		Type:        "RESOURCE",
		Description: "Binance public API for price, klines, orderbook, and 24hr stats",
	}}
}

func skills() []types.AgentSkillCard {
	return []types.AgentSkillCard{{
		ID:          "crypto.price",
		Name:        "Crypto Price Query",
		Description: "Query real-time and historical crypto prices from Binance",
		Tags:        []string{"crypto", "binance", "price", "trading"},
		Examples:    []string{"BTCUSDT price", "ETH 24h change", "SOL klines 30", "BNB orderbook 5"},
	}}
}

// skillConfigs is the registration-side form of the agent's skills.
func skillConfigs() []types.SkillConfig {
	return []types.SkillConfig{{
		Name:        "Crypto Price Query",
		Description: "Query real-time and historical crypto prices from Binance",
	}}
}

func main() {
	mode := "auto"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	polling := strings.HasPrefix(mode, "polling")
	manual := strings.HasSuffix(mode, "manual")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Agent SDK Startup Guide — Binance Price Agent (mode=%s)\n", mode)

	token, wallet, err := exauth.EnsureAuth(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth error:", err)
		os.Exit(1)
	}
	if wallet == "" {
		wallet = getenv("MEMBASE_ACCOUNT", "0x5ea13664c5ce67753f208540d25b913788aa3daa")
	}

	aipEndpoint := getenv("AIP_ENDPOINT", "https://api.aip.unibase.com")
	gatewayURL := getenv("GATEWAY_URL", "https://gateway.aip.unibase.com")
	publicURL := getenv("AGENT_PUBLIC_URL", "http://your-public-ip:8200")
	port, _ := strconv.Atoi(getenv("AGENT_PORT", "8200"))

	handle := "binance_price"
	name := "Binance Price Agent"
	description := "Real-time cryptocurrency price queries via Binance public API"

	endpointURL := publicURL
	if polling {
		endpointURL = "" // POLLING mode
	}

	costBase := 0.0
	costModel := &types.CostModel{BaseCallFee: &costBase}

	// Manual modes register explicitly before starting the service.
	if manual {
		fmt.Println("Manual registration: POST /agents/register")
		cfg := types.AgentConfig{
			Name: name, Handle: handle, Description: description,
			EndpointURL: endpointURL, Skills: skillConfigs(), CostModel: *costModel,
			Currency: "USD", ChainID: 97,
			Metadata:     map[string]any{"chain_id": 97, "author": "Unibase Demo"},
			JobOfferings: jobOfferings(), JobResources: jobResources(),
		}
		if _, err := platform.New(aipEndpoint).RegisterAgent(ctx, cfg, wallet, token); err != nil {
			fmt.Fprintln(os.Stderr, "registration failed:", err)
			os.Exit(1)
		}
	}

	agent := &BinancePriceAgent{http: &http.Client{Timeout: 10 * time.Second}}

	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:                name,
		Handle:              handle,
		Description:         description,
		Host:                "0.0.0.0",
		Port:                port,
		Skills:              skills(),
		UserID:              "user:" + strings.TrimPrefix(wallet, "user:"),
		PrivyToken:          token,
		AIPEndpoint:         aipEndpoint,
		GatewayURL:          gatewayURL,
		EndpointURL:         endpointURL,
		ViaGateway:          polling, // job-queue discovery for private agents
		DisableAutoRegister: manual,
		CostModel:           costModel,
		ChainID:             97,
		JobOfferings:        jobOfferings(),
		JobResources:        jobResources(),
		Metadata:            map[string]any{"mode": modeTag(polling)},
	}, agent.Handle, nil)

	fmt.Printf("Mode: %s | Port: %d | Endpoint: %q\n", strings.ToUpper(modeTag(polling)), port, endpointURL)
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

func modeTag(polling bool) string {
	if polling {
		return "polling"
	}
	return "push"
}
