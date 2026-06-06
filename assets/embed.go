// Package assets embeds BizHawk Lua and holds non-Go files (plugins/) used at runtime.
package assets

import _ "embed"

// ServerLua is the BizHawk IPC bridge (server.lua).
//
//go:embed server.lua
var ServerLua []byte

// LuaSocketCoreLinuxAMD64 is defined in luasocket_linux_amd64.go (linux/amd64) or luasocket_stub.go.
