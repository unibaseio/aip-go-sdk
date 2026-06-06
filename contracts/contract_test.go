// Package contracts holds the cross-language wire contract for the Unibase AIP
// SDK. The golden JSON files under fixtures/ are the single source of truth that
// BOTH the Go and Python SDKs must serialize to / deserialize from identically.
//
// This Go test asserts that the canonical Go values serialize to JSON that is
// semantically equal to each fixture. The Python SDK should ship a mirror test
// over the same fixture files. When the contract changes intentionally,
// regenerate the fixtures with:
//
//	UPDATE_FIXTURES=1 go test ./contracts/
package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unibaseio/aip-go-sdk/a2a"
	"github.com/unibaseio/aip-go-sdk/messaging"
	"github.com/unibaseio/aip-go-sdk/types"
)

// fixedPrice is a helper for a *float64 literal.
func fixedPrice(v float64) *float64 { return &v }

// jobOffering is the canonical job offering used across fixtures.
func jobOffering() types.AgentJobOffering {
	return types.AgentJobOffering{
		ID:          "yes_no_probability",
		Name:        "yes_no_probability",
		Description: "Estimates YES/NO probabilities for any prediction market topic.",
		Type:        "JOB",
		Price:       0,
		PriceV2:     map[string]any{"type": "fixed", "amount": 0.0015, "currency": "USDC"},
		JobInput:    "Will BTC break $150k by end of 2026?",
		JobOutput:   "Topic: ...\nYES: <0-100>%\nNO: <0-100>%\nReasoning: ...",
		Requirement: map[string]any{
			"type": "object", "required": []any{"topic"},
			"properties": map[string]any{"topic": map[string]any{"type": "string"}},
		},
		Deliverable: map[string]any{
			"type": "object", "required": []any{"text"},
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
		},
		SLAMinutes: 1,
		Active:     true,
	}
}

// agentConfig is the canonical AgentConfig used for registration/card fixtures.
func agentConfig() types.AgentConfig {
	return types.AgentConfig{
		Name:         "Prediction Market Agent",
		Description:  "Estimates YES/NO probabilities.",
		Handle:       "prediction_market_demo",
		Capabilities: []string{"streaming"},
		Skills: []types.SkillConfig{{
			Name:        "YES/NO Probability",
			Description: "Estimate the YES/NO probability of any topic.",
		}},
		CostModel:    types.CostModel{BaseCallFee: fixedPrice(0.0015)},
		Currency:     "USD",
		Metadata:     map[string]any{"version": "1.0.0", "mode": "private"},
		EndpointURL:  "",
		JobOfferings: []types.AgentJobOffering{jobOffering()},
		ChainID:      97,
	}
}

// aipMetadata is the canonical AIP metadata envelope.
func aipMetadata() map[string]any {
	m := &messaging.AIPMetadata{
		RunID:             "run-123",
		CallerID:          "user:0xabc",
		CallerChain:       []string{"user:0xabc"},
		ConversationID:    "conv-1",
		PaymentAuthorized: true,
		Custom:            map[string]any{"topic": "Will BTC break $150k?"},
	}
	return m.ToMap()
}

// a2aMessage is the canonical A2A message (locks role/kind/camelCase wire form).
func a2aMessage() *a2a.Message {
	return &a2a.Message{
		ID:        "msg-1",
		ContextID: "ctx-1",
		Role:      a2a.RoleUser,
		Parts: a2a.ContentParts{
			a2a.NewTextPart("hello"),
			a2a.NewDataPart(map[string]any{"k": "v"}),
		},
	}
}

func TestContracts(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"agent_registration", agentConfig().ToRegistrationMap()},
		{"agent_card", agentConfig().ToAgentCard("42", "0xRegistry")},
		{"job_offering", jobOffering()},
		{"aip_metadata", aipMetadata()},
		{"a2a_message", a2aMessage()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toNormalizedJSON(t, tc.value)
			path := filepath.Join("fixtures", tc.name+".json")

			if os.Getenv("UPDATE_FIXTURES") == "1" {
				writeFixture(t, path, tc.value)
				return
			}

			want := readFixture(t, path)
			if !reflect.DeepEqual(got, want) {
				gotPretty, _ := json.MarshalIndent(got, "", "  ")
				wantPretty, _ := json.MarshalIndent(want, "", "  ")
				t.Fatalf("contract drift for %s\n--- got ---\n%s\n--- want (fixture) ---\n%s",
					tc.name, gotPretty, wantPretty)
			}
		})
	}
}

// toNormalizedJSON marshals v then unmarshals into a generic value, so that
// comparisons ignore key order and formatting.
func toNormalizedJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func readFixture(t *testing.T, path string) any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v (regenerate with UPDATE_FIXTURES=1)", path, err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("invalid fixture %s: %v", path, err)
	}
	return out
}

func writeFixture(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("wrote %s", path)
}
