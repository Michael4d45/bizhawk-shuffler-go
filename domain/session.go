package domain

import (
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func FreshServerState() protocol.ServerState {
	st := protocol.DefaultServerState()
	st.SwapEnabled = true
	st.MaxIntervalSecs = 300
	st.MinIntervalSecs = 5
	st.GameSwapInstances = []protocol.GameSwapInstance{}
	st.Games = []string{}
	st.MainGames = []protocol.GameEntry{}
	st.Plugins = map[string]protocol.Plugin{}
	return st
}

type ServerSession struct {
	state protocol.ServerState
}

func NewServerSession(initial *protocol.ServerState) *ServerSession {
	st := FreshServerState()
	if initial != nil {
		st = cloneState(*initial)
	}
	return &ServerSession{state: st}
}

func (s *ServerSession) Snapshot() protocol.ServerState {
	return cloneState(s.state)
}

func (s *ServerSession) Raw() *protocol.ServerState {
	return &s.state
}

func (s *ServerSession) Update(mutator func(*protocol.ServerState)) string {
	mutator(&s.state)
	updatedAt := time.Now()
	s.state.UpdatedAt = updatedAt
	return updatedAt.Format(time.RFC3339Nano)
}

func (s *ServerSession) SnapshotPlayers() map[string]protocol.Player {
	out := make(map[string]protocol.Player, len(s.state.Players))
	for k, v := range s.state.Players {
		out[k] = v
	}
	return out
}

func (s *ServerSession) SnapshotGames() (games []string, mainGames []protocol.GameEntry, instances []protocol.GameSwapInstance) {
	games = append([]string(nil), s.state.Games...)
	mainGames = append([]protocol.GameEntry(nil), s.state.MainGames...)
	instances = append([]protocol.GameSwapInstance(nil), s.state.GameSwapInstances...)
	return games, mainGames, instances
}

func (s *ServerSession) GetPlayer(name string) protocol.Player {
	if p, ok := s.state.Players[name]; ok {
		return p
	}
	return protocol.Player{Name: name, HasFiles: false, Connected: false, BizhawkReady: false}
}

func (s *ServerSession) SetState(next protocol.ServerState) {
	s.state = cloneState(next)
}

func cloneState(st protocol.ServerState) protocol.ServerState {
	out := st
	if st.Players != nil {
		out.Players = make(map[string]protocol.Player, len(st.Players))
		for k, v := range st.Players {
			out.Players[k] = v
		}
	}
	if st.Plugins != nil {
		out.Plugins = make(map[string]protocol.Plugin, len(st.Plugins))
		for k, v := range st.Plugins {
			out.Plugins[k] = v
		}
	}
	out.Games = append([]string(nil), st.Games...)
	out.MainGames = append([]protocol.GameEntry(nil), st.MainGames...)
	out.GameSwapInstances = append([]protocol.GameSwapInstance(nil), st.GameSwapInstances...)
	return out
}
