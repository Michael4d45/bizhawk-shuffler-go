package server

// TODO: Implement LAN server discovery broadcaster
// - Add DiscoveryBroadcaster struct with UDP connection
// - Implement Start() method to begin periodic broadcasting
// - Implement Stop() method to clean up UDP connection
// - Broadcast server info (host, port, name) via UDP multicast
// - Handle broadcast errors gracefully
// - Make broadcast interval configurable

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// DiscoveryBroadcaster handles UDP broadcasting of server presence
type DiscoveryBroadcaster struct {
	config     *types.DiscoveryConfig
	serverHost string
	serverPort int
	serverName string
	conn       *net.UDPConn
	running    bool
	mu         sync.Mutex
	cancel     context.CancelFunc
}

// NewDiscoveryBroadcaster creates a new discovery broadcaster
func NewDiscoveryBroadcaster(config *types.DiscoveryConfig, serverHost string, serverPort int, serverName string) *DiscoveryBroadcaster {
	// TODO: Initialize broadcaster with configuration
	return &DiscoveryBroadcaster{
		config:     config,
		serverHost: serverHost,
		serverPort: serverPort,
		serverName: serverName,
	}
}

// Start begins broadcasting server presence
func (db *DiscoveryBroadcaster) Start(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.running {
		return nil // Already running
	}

	// Set up multicast connection
	if err := db.setupMulticastConnection(); err != nil {
		return err
	}

	db.running = true
	ctx, db.cancel = context.WithCancel(ctx)

	// Start broadcasting goroutine
	go db.broadcastLoop(ctx)

	log.Printf("[DiscoveryBroadcaster] Started broadcasting on %s", db.config.MulticastAddress)
	return nil
}

// Stop stops broadcasting and cleans up resources
func (db *DiscoveryBroadcaster) Stop() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.running {
		return nil
	}

	db.running = false
	if db.cancel != nil {
		db.cancel()
	}

	if db.conn != nil {
		if err := db.conn.Close(); err != nil {
			log.Printf("[DiscoveryBroadcaster] Error closing connection: %v", err)
		}
		db.conn = nil
	}

	log.Printf("[DiscoveryBroadcaster] Stopped broadcasting")
	return nil
}

// broadcastLoop runs the periodic broadcasting
func (db *DiscoveryBroadcaster) broadcastLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(db.config.BroadcastIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := db.broadcast(); err != nil {
				log.Printf("[DiscoveryBroadcaster] Broadcast error: %v", err)
			}
		}
	}
}

// broadcast sends a single discovery message
func (db *DiscoveryBroadcaster) broadcast() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.running || db.conn == nil {
		return nil
	}

	msg := types.NewDiscoveryMessage(db.serverHost, db.serverPort, db.serverName)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Send multicast message
	_, err = db.conn.Write(data)
	if err != nil {
		log.Printf("[DiscoveryBroadcaster] Multicast send error: %v", err)
	}

	// Also send unicast message to localhost for same-machine communication
	localAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1901")
	if err != nil {
		log.Printf("[DiscoveryBroadcaster] Failed to resolve localhost:1901: %v", err)
	} else {
		localConn, err := net.DialUDP("udp", nil, localAddr)
		if err != nil {
			log.Printf("[DiscoveryBroadcaster] Failed to create local connection: %v", err)
		} else {
			_, err = localConn.Write(data)
			localConn.Close()
			if err != nil {
				log.Printf("[DiscoveryBroadcaster] Unicast send error: %v", err)
			}
		}
	}

	return nil
}

// setupMulticastConnection sets up the UDP multicast connection
func (db *DiscoveryBroadcaster) setupMulticastConnection() error {
	addr, err := net.ResolveUDPAddr("udp", db.config.MulticastAddress)
	if err != nil {
		return err
	}

	// Try to find the interface that matches the server host
	var localAddr *net.UDPAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Printf("[DiscoveryBroadcaster] Failed to get interfaces: %v", err)
	} else {
		for _, i := range ifaces {
			addrs, err := i.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				ip := ipNet.IP.To4()
				if ip != nil && ip.String() == db.serverHost {
					localAddr = &net.UDPAddr{IP: ip, Port: 0}
					log.Printf("[DiscoveryBroadcaster] Found matching interface: %s (%s)", i.Name, ip.String())
					break
				}
			}
			if localAddr != nil {
				break
			}
		}
	}

	if localAddr == nil {
		// Fallback to server host
		localAddr = &net.UDPAddr{IP: net.ParseIP(db.serverHost), Port: 0}
	}

	conn, err := net.DialUDP("udp", localAddr, addr)
	if err != nil {
		return err
	}

	log.Printf("[DiscoveryBroadcaster] Broadcasting from %s to %s", conn.LocalAddr(), addr)
	db.conn = conn
	return nil
}
