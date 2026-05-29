package protocol

import (
	"encoding/json"
	"fmt"
)

var clientToServer = map[CommandName]bool{
	CmdHello: true, CmdAck: true, CmdNack: true, CmdGamesUpdateAck: true,
	CmdStatusUpdate: true, CmdTypeLua: true, CmdConfigResponse: true, CmdHelloAdmin: true,
}

var serverToClient = map[CommandName]bool{
	CmdPing: true, CmdResume: true, CmdPause: true, CmdSwap: true, CmdMessage: true,
	CmdGamesUpdate: true, CmdClearSaves: true, CmdRequestSave: true, CmdPluginReload: true,
	CmdFullscreenToggle: true, CmdCheckConfig: true, CmdUpdateConfig: true, CmdStateUpdate: true,
}

func EncodeCommand(cmd Command) (string, error) {
	b, err := json.Marshal(cmd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeCommand(raw string) (Command, error) {
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		return Command{}, err
	}
	if cmd.Cmd == "" {
		return Command{}, fmt.Errorf("missing cmd")
	}
	return cmd, nil
}

func IsClientToServer(cmd CommandName) bool {
	return clientToServer[cmd]
}

func IsServerToClient(cmd CommandName) bool {
	return serverToClient[cmd]
}
