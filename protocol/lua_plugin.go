package protocol

import "fmt"

// ParseLuaPluginCommand parses Lua CMD lines for plugin-originated kinds only.
func ParseLuaPluginCommand(line string) (*LuaCommand, error) {
	cmd, err := ParseLuaCommand(line)
	if err != nil {
		return nil, err
	}
	switch cmd.Kind {
	case LuaCmdSwap, LuaCmdSwapMe, LuaCmdMessage:
		return cmd, nil
	default:
		return nil, fmt.Errorf("unknown lua kind: %s", cmd.Kind)
	}
}
