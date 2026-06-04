// Command private_agent runs a calculator agent in Gateway POLLING mode,
// mirroring the Python examples/private_agent_full.py. With no public endpoint
// set, the agent polls the Gateway for tasks — suitable for hosts behind a
// firewall/NAT.
package main

import (
	"context"
	"fmt"
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

// CalculatorAgent evaluates simple arithmetic expressions.
type CalculatorAgent struct{}

func (c *CalculatorAgent) Handle(ctx context.Context, input string) (string, error) {
	fmt.Printf("[Calculator Agent] request: %s\n", input)
	expr := stripPrefixes(strings.ToLower(strings.TrimSpace(input)))
	result, err := eval(expr)
	if err != nil {
		return fmt.Sprintf("Calculation Error: %v\nPlease provide a valid expression.", err), nil
	}
	return fmt.Sprintf("Calculation Result\nExpression: %s\nResult: %g\n", expr, result), nil
}

func stripPrefixes(s string) string {
	for _, p := range []string{"calculate", "compute", "what is", "what's", "solve", "evaluate", "find", "tell me"} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(s[len(p):])
		}
	}
	return s
}

func main() {
	userWallet := getenv("MEMBASE_ACCOUNT", "0x5ea13664c5ce67753f208540d25b913788aa3daa")
	aipEndpoint := getenv("AIP_ENDPOINT", "http://localhost:8001")
	gatewayURL := getenv("GATEWAY_URL", "http://localhost:8080")
	host := getenv("AGENT_HOST", "0.0.0.0")
	port, _ := strconv.Atoi(getenv("AGENT_PORT", "8201"))

	base := 0.0005
	costModel := &types.CostModel{BaseCallFee: &base}

	skills := []types.AgentSkillCard{{
		ID:          "calculator.compute",
		Name:        "Mathematical Computation",
		Description: "Perform mathematical calculations",
		Tags:        []string{"math", "calculator", "computation"},
		Examples:    []string{"Calculate 25 * 4 + 10", "Compute (2 + 3) * 4"},
	}}

	agent := &CalculatorAgent{}

	// POLLING mode: EndpointURL is left empty, so the server polls the Gateway
	// for tasks instead of exposing a public endpoint.
	srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
		Name:        "Private Calculator Agent",
		Description: "A private calculator agent in a secure environment",
		Host:        host,
		Port:        port,
		Skills:      skills,
		UserID:      "user:" + userWallet,
		AIPEndpoint: aipEndpoint,
		GatewayURL:  gatewayURL,
		Handle:      "calculator_private",
		CostModel:   costModel,
		EndpointURL: "", // POLLING mode
		Metadata:    map[string]any{"version": "1.0.0", "mode": "private", "deployment": "gateway_polling"},
	}, agent.Handle, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Private Calculator Agent (internal port %d), polling Gateway at %s\n", port, gatewayURL)
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

// --- minimal arithmetic evaluator: + - * / and parentheses ---

type parser struct {
	s   string
	pos int
}

func eval(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}
	p := &parser{s: expr}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.s) {
		return 0, fmt.Errorf("unexpected token at %d", p.pos)
	}
	return v, nil
}

func (p *parser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
		op := p.s[p.pos]
		p.pos++
		rhs, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += rhs
		} else {
			v -= rhs
		}
	}
	return v, nil
}

func (p *parser) parseTerm() (float64, error) {
	v, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.s) && (p.s[p.pos] == '*' || p.s[p.pos] == '/') {
		op := p.s[p.pos]
		p.pos++
		rhs, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= rhs
		}
	}
	return v, nil
}

func (p *parser) parseFactor() (float64, error) {
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	switch c := p.s[p.pos]; {
	case c == '(':
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing paren")
		}
		p.pos++
		return v, nil
	case c == '-':
		p.pos++
		v, err := p.parseFactor()
		return -v, err
	default:
		start := p.pos
		for p.pos < len(p.s) && (p.s[p.pos] >= '0' && p.s[p.pos] <= '9' || p.s[p.pos] == '.') {
			p.pos++
		}
		if start == p.pos {
			return 0, fmt.Errorf("invalid token at %d", p.pos)
		}
		return strconv.ParseFloat(p.s[start:p.pos], 64)
	}
}
