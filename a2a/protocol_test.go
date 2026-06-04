package a2a

import (
	"encoding/json"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	msg := NewMessage(RoleUser, "m1", "hello")
	msg.Parts = append(msg.Parts, NewDataPart(map[string]any{"k": "v"}))

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(string(b), `"messageId":"m1"`) {
		t.Fatalf("expected camelCase messageId, got %s", b)
	}
	if !contains(string(b), `"role":"user"`) {
		t.Fatalf("expected spec role value, got %s", b)
	}
	if !contains(string(b), `"kind":"text"`) {
		t.Fatalf("expected kind discriminator, got %s", b)
	}

	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if GetMessageText(&got) != "hello" {
		t.Fatalf("text = %q, want hello", GetMessageText(&got))
	}
	data := GetMessageData(&got)
	if data["k"] != "v" {
		t.Fatalf("data = %v, want k=v", data)
	}
}

func TestPartHelpers(t *testing.T) {
	if txt, ok := PartText(NewTextPart("hi")); !ok || txt != "hi" {
		t.Fatalf("PartText = %q,%v", txt, ok)
	}
	if data, ok := PartData(NewDataPart(map[string]any{"a": 1})); !ok || data["a"] != 1 {
		t.Fatalf("PartData = %v,%v", data, ok)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
