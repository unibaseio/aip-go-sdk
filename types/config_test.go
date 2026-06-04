package types

import (
	"encoding/json"
	"testing"
)

func TestAgentConfigToAgentCard(t *testing.T) {
	base := 0.05
	cfg := AgentConfig{
		Name:        "My Agent",
		Description: "does things",
		CostModel:   CostModel{BaseCallFee: &base},
		Currency:    "USD",
		Skills:      []SkillConfig{{Name: "calc", Description: "calculates"}},
		ChainID:     97,
	}
	card := cfg.ToAgentCard("", "")

	if card.Type != AgentCardType {
		t.Fatalf("type = %q", card.Type)
	}
	if card.Name != "My Agent" {
		t.Fatalf("name = %q", card.Name)
	}
	if len(card.Skills) != 1 || card.Skills[0].Name != "calc" {
		t.Fatalf("skills = %+v", card.Skills)
	}
	if cfg.Price() != 0.05 {
		t.Fatalf("price = %v, want 0.05", cfg.Price())
	}

	b, _ := json.Marshal(card)
	if !jsonHasKey(b, "jobOfferings") {
		t.Fatalf("expected camelCase jobOfferings in %s", b)
	}
}

func TestAgentConfigPriceDefault(t *testing.T) {
	cfg := AgentConfig{Name: "x"}
	if cfg.Price() != 0.001 {
		t.Fatalf("default price = %v, want 0.001", cfg.Price())
	}
}

func jsonHasKey(b []byte, key string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
