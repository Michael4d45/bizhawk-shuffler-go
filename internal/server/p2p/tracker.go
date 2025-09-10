package p2p

import (
	"log"
	"net/netip"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// SaveTracker is an in-memory tracker mapping save state instance IDs to peers that
// claim to possess that save. This is a minimal first implementation intended for
// whole‑file distribution (no piece exchange yet). It is designed to run entirely
// in memory; if it fails we fall back to HTTP downloads only. All methods are safe
// for concurrent use.
type SaveTracker struct {
	mu         sync.RWMutex
	byInstance map[string]map[string]*peerEntry // instanceID -> peerID -> entry
	peers      map[string]*peerEntry            // peerID -> entry (for quick cleanup)
	ttl        time.Duration                    // inactivity TTL until a peer entry expires
	sweepEvery time.Duration
	lastSweep  time.Time
}

type peerEntry struct {
	peerID    string
	address   string                      // best-effort remote address string
	instances map[string]AnnounceInstance // instanceID -> announced metadata
	lastSeen  time.Time
}

// AnnounceInstance represents one save state a peer claims to seed.
type AnnounceInstance struct {
	InstanceID string `json:"instance_id"`
	Hash       string `json:"hash,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// NewSaveTracker creates a tracker with the provided peer inactivity TTL and sweep interval.
func NewSaveTracker(ttl, sweepEvery time.Duration) *SaveTracker {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	if sweepEvery <= 0 || sweepEvery > ttl {
		sweepEvery = ttl / 2
		if sweepEvery < 5*time.Second {
			sweepEvery = 5 * time.Second
		}
	}
	st := &SaveTracker{
		byInstance: make(map[string]map[string]*peerEntry),
		peers:      make(map[string]*peerEntry),
		ttl:        ttl,
		sweepEvery: sweepEvery,
		lastSweep:  time.Now(),
	}
	return st
}

// Announce registers (or refreshes) a peer's participation for the listed save state
// instances. Missing or empty instance list keeps the peer alive but does not seed anything.
func (t *SaveTracker) Announce(peerID string, remoteAddr string, insts []AnnounceInstance) {
	if peerID == "" {
		return // invalid announce; caller should have logged already (expected failure)
	}
	now := time.Now()
	t.mu.Lock()
	pe := t.peers[peerID]
	if pe == nil {
		pe = &peerEntry{peerID: peerID, address: remoteAddr, instances: make(map[string]AnnounceInstance), lastSeen: now}
		t.peers[peerID] = pe
	}
	pe.lastSeen = now
	// Clear previous instance mapping for this peer (simple strategy; could diff later)
	for instID := range pe.instances {
		if bucket, ok := t.byInstance[instID]; ok {
			delete(bucket, peerID)
			if len(bucket) == 0 {
				delete(t.byInstance, instID)
			}
		}
	}
	pe.instances = make(map[string]AnnounceInstance, len(insts))
	for _, ai := range insts {
		if ai.InstanceID == "" {
			continue
		}
		pe.instances[ai.InstanceID] = ai
		bucket := t.byInstance[ai.InstanceID]
		if bucket == nil {
			bucket = make(map[string]*peerEntry)
			t.byInstance[ai.InstanceID] = bucket
		}
		bucket[peerID] = pe
	}
	sweepNeeded := now.Sub(t.lastSweep) >= t.sweepEvery
	t.mu.Unlock()
	if sweepNeeded {
		go t.sweep()
	}
}

// sweep removes expired peers. It is safe to run concurrently; only one will hold the lock.
func (t *SaveTracker) sweep() {
	now := time.Now()
	t.mu.Lock()
	if now.Sub(t.lastSweep) < t.sweepEvery/2 { // another sweep just happened
		t.mu.Unlock()
		return
	}
	t.lastSweep = now
	cutoff := now.Add(-t.ttl)
	removedPeers := 0
	for pid, pe := range t.peers {
		if pe.lastSeen.Before(cutoff) {
			// Remove from instance buckets first
			for instID := range pe.instances {
				if bucket, ok := t.byInstance[instID]; ok {
					delete(bucket, pid)
					if len(bucket) == 0 {
						delete(t.byInstance, instID)
					}
				}
			}
			delete(t.peers, pid)
			removedPeers++
		}
	}
	t.mu.Unlock()
	if removedPeers > 0 {
		log.Printf("[P2P][tracker] swept expired peers removed=%d", removedPeers)
	}
}

// Peers returns a slice of PeerInfo advertising the given instance.
func (t *SaveTracker) Peers(instanceID string) []types.PeerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	bucket := t.byInstance[instanceID]
	if bucket == nil {
		return nil
	}
	out := make([]types.PeerInfo, 0, len(bucket))
	for _, pe := range bucket {
		instList := make([]string, 0, len(pe.instances))
		for id := range pe.instances {
			instList = append(instList, id)
		}
		out = append(out, types.PeerInfo{ID: pe.peerID, Addr: pe.address, LastSeen: pe.lastSeen.Unix(), Instances: instList})
	}
	return out
}

// Snapshot returns a summary map for debugging/logging.
func (t *SaveTracker) Snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	perInstance := make(map[string]int, len(t.byInstance))
	for inst, bucket := range t.byInstance {
		perInstance[inst] = len(bucket)
	}
	return map[string]any{
		"peers_total":        len(t.peers),
		"instances_tracked":  len(t.byInstance),
		"seeds_per_instance": perInstance,
		"ttl_seconds":        int(t.ttl.Seconds()),
	}
}

// ParseRemoteAddr attempts to convert the remote address into an IP:port string.
func ParseRemoteAddr(raw string) string {
	if raw == "" {
		return ""
	}
	// Attempt to parse using netip (handles IPv4/IPv6); ignore errors (return raw best effort).
	hostPort, err := netip.ParseAddrPort(raw)
	if err != nil {
		return raw
	}
	return hostPort.String()
}
