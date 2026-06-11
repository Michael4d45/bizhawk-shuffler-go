package serverhost

import (
	"hash/fnv"
	"strings"
)

// playerNameHashIndex maps a player name to a stable index in [0, instanceCount).
func playerNameHashIndex(playerName string, instanceCount int) int {
	if instanceCount <= 0 {
		return 0
	}
	normalized := strings.ToLower(strings.TrimSpace(playerName))
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalized))
	return int(h.Sum64() % uint64(instanceCount))
}
