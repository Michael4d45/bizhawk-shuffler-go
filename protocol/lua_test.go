package protocol

import "testing"

func TestParseLuaPluginSwapMe(t *testing.T) {
	line := "CMD|swap_me|message=Memory Tracker: door 32792 -> 32785"
	cmd, err := ParseLuaPluginCommand(line)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != LuaCmdSwapMe {
		t.Fatal(cmd.Kind)
	}
	if cmd.Fields["message"] == "" {
		t.Fatal("missing message field")
	}
}

func TestParseLuaPluginRejectsSave(t *testing.T) {
	_, err := ParseLuaPluginCommand("CMD|1738123|SAVE|instance-1")
	if err == nil {
		t.Fatal("expected error")
	}
}
