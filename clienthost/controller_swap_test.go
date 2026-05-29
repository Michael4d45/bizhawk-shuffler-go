package clienthost

import "testing"

func TestPayloadBool(t *testing.T) {
	if !payloadBool(map[string]any{"skip_save": true}, "skip_save") {
		t.Fatal("expected true")
	}
	if payloadBool(map[string]any{"skip_save": false}, "skip_save") {
		t.Fatal("expected false")
	}
	if payloadBool(map[string]any{}, "skip_save") {
		t.Fatal("expected missing key false")
	}
}
