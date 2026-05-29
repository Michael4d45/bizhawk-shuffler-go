package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

func SelectNextGame(games []string, exclude map[string]bool, seed int) (game string, nextSeed int, ok bool) {
	var filtered []string
	for _, g := range games {
		if !exclude[g] {
			filtered = append(filtered, g)
		}
	}
	if len(filtered) == 0 {
		return "", seed, false
	}
	idx := absInt(seed) % len(filtered)
	return filtered[idx], seed + 1, true
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func GenerateInstanceID(game string, existing map[string]bool) string {
	base := game
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[:i]
	}
	base = nonAlnum.ReplaceAllString(base, "-")
	base = strings.ToLower(strings.Trim(base, "-"))
	if base == "" {
		base = "instance"
	}
	if len(base) > 20 {
		base = base[:20]
	}
	id := base
	n := 1
	for existing[id] {
		id = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	return id
}

func CategorizeInstances(
	instances []GameSwapInstance,
	players map[string]Player,
	_, currentGame, currentInstance string,
	preventSame bool,
) [][]GameSwapInstance {
	buckets := make([][]GameSwapInstance, 6)
	for i := range buckets {
		buckets[i] = []GameSwapInstance{}
	}
	for _, inst := range instances {
		var owner *Player
		for _, p := range players {
			if p.InstanceID == inst.ID {
				cp := p
				owner = &cp
				break
			}
		}
		unassigned := owner == nil
		sameGame := inst.Game == currentGame
		sameInst := inst.ID == currentInstance
		var bucket int
		if preventSame {
			switch {
			case unassigned && !sameGame:
				bucket = 0
			case unassigned && sameGame:
				bucket = 1
			case !unassigned && !sameGame:
				bucket = 2
			case !unassigned && sameGame && !sameInst:
				bucket = 3
			default:
				bucket = 4
			}
		} else if unassigned {
			bucket = 0
		} else {
			bucket = 2
		}
		buckets[bucket] = append(buckets[bucket], inst)
	}
	return buckets
}

func SetupSyncState(state ServerState) ServerState {
	games := make(map[string]bool)
	for _, g := range state.Games {
		games[g] = true
	}
	for _, mg := range state.MainGames {
		games[mg.File] = true
	}
	out := make([]string, 0, len(games))
	for g := range games {
		out = append(out, g)
	}
	state.Games = out
	return state
}

func SetupSaveState(state ServerState) ServerState {
	instances := append([]GameSwapInstance(nil), state.GameSwapInstances...)
	existingGames := make(map[string]bool)
	ids := make(map[string]bool)
	for _, inst := range instances {
		existingGames[inst.Game] = true
		ids[inst.ID] = true
	}
	for _, mg := range state.MainGames {
		if !existingGames[mg.File] {
			id := GenerateInstanceID(mg.File, ids)
			ids[id] = true
			instances = append(instances, GameSwapInstance{
				ID: id, Game: mg.File, FileState: FileStateNone,
			})
		}
	}
	state.GameSwapInstances = instances
	return state
}
