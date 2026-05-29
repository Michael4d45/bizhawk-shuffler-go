package protocol

import "testing"

func TestParseSettingsMeta(t *testing.T) {
	meta := ParseSettingsMeta(map[string]string{
		"name":                          "memory-tracker",
		"setting.command_type.type":     "dropdown",
		"setting.command_type.options":  "swap,swap_me",
		"setting.enabled_types.type":    "multiselect",
		"setting.enabled_types.options": "door,health",
	})
	if meta["command_type"].Type != "dropdown" {
		t.Fatal(meta["command_type"])
	}
	if len(meta["command_type"].Options) != 2 {
		t.Fatal(meta["command_type"].Options)
	}
	if len(ParseSettingsMeta(map[string]string{"version": "1.0.0"})) != 0 {
		t.Fatal("expected empty")
	}
}
