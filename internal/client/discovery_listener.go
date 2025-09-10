package client

// TODO: Implement LAN server discovery listener
// - Add DiscoveryListener struct with UDP connection
// - Implement Start() method to begin listening for broadcasts
// - Implement Stop() method to clean up UDP connection
// - Collect discovered servers in a map or slice
// - Handle incoming UDP messages and parse server info
// - Provide method to get list of discovered servers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// DiscoveryListener handles receiving UDP broadcasts from servers
type DiscoveryListener struct {
	config        *types.DiscoveryConfig
	discovered    map[string]*types.ServerInfo
	mu            sync.RWMutex
	conn          *net.UDPConn
	localConn     *net.UDPConn // Store localhost connection for cleanup
	running       bool
	cancel        context.CancelFunc
	onServerFound func(*types.ServerInfo) // Callback for when a server is discovered
}

// NewDiscoveryListener creates a new discovery listener
func NewDiscoveryListener(config *types.DiscoveryConfig) *DiscoveryListener {
	// TODO: Initialize listener with configuration
	return &DiscoveryListener{
		config:     config,
		discovered: make(map[string]*types.ServerInfo),
	}
}

// Start begins listening for server broadcasts
func (dl *DiscoveryListener) Start(ctx context.Context) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if dl.running {
		return nil // Already running
	}

	dl.running = true
	ctx, dl.cancel = context.WithCancel(ctx)

	// Set up multicast connection
	if err := dl.setupMulticastConnection(ctx); err != nil {
		return err
	}

	// Start listening goroutine
	go dl.listen(ctx)

	log.Printf("[DiscoveryListener] Started listening on %s at %s", dl.config.MulticastAddress, time.Now().Format("15:04:05.000"))
	return nil
}

// Stop stops listening and cleans up resources
func (dl *DiscoveryListener) Stop() error {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if !dl.running {
		return nil
	}

	dl.running = false
	if dl.cancel != nil {
		dl.cancel()
	}

	if dl.conn != nil {
		if err := dl.conn.Close(); err != nil {
			log.Printf("[DiscoveryListener] Error closing multicast connection: %v", err)
		}
		dl.conn = nil
	}

	if dl.localConn != nil {
		if err := dl.localConn.Close(); err != nil {
			log.Printf("[DiscoveryListener] Error closing localhost connection: %v", err)
		}
		dl.localConn = nil
	}

	log.Printf("[DiscoveryListener] Stopped listening at %s", time.Now().Format("15:04:05.000"))
	return nil
}

// GetDiscoveredServers returns a copy of all discovered servers
func (dl *DiscoveryListener) GetDiscoveredServers() []*types.ServerInfo {
	dl.mu.RLock()
	defer dl.mu.RUnlock()

	servers := make([]*types.ServerInfo, 0, len(dl.discovered))
	maxAge := time.Duration(dl.config.ListenTimeoutSec) * time.Second

	for _, server := range dl.discovered {
		if !server.IsExpired(maxAge) {
			servers = append(servers, server)
		}
	}

	return servers
}

// listen handles incoming UDP packets
func (dl *DiscoveryListener) listen(ctx context.Context) {
	buffer := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// TODO: Set read deadline
			if err := dl.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				log.Printf("Failed to set read deadline: %v", err)
				continue
			}
			n, addr, err := dl.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is expected, continue listening
				}
				log.Printf("UDP read error: %v", err)
				continue
			}

			log.Printf("[DiscoveryListener] Received %d bytes from %s", n, addr)
			log.Printf("[DiscoveryListener] Raw data: %q", string(buffer[:n]))

			// Try to parse as JSON discovery message
			var msg types.DiscoveryMessage
			if err := json.Unmarshal(buffer[:n], &msg); err != nil {
				log.Printf("[DiscoveryListener] Failed to parse message from %s as JSON: %v", addr, err)
				continue
			}

			// Validate that this is a BizShuffle server message
			if msg.Type != "bizshuffle_server" {
				log.Printf("[DiscoveryListener] Ignoring non-BizShuffle message from %s (type: %s)", addr, msg.Type)
				continue
			}

			log.Printf("[DiscoveryListener] Received BizShuffle server message from %s: server=%s (%s:%d)", addr, msg.ServerName, msg.Host, msg.Port)

			// TODO: Validate message
			if !msg.IsValid() {
				continue
			}

			// TODO: Create server info
			serverInfo := &types.ServerInfo{
				Name:     msg.ServerName,
				Host:     msg.Host,
				Port:     msg.Port,
				Version:  msg.Version,
				LastSeen: time.Now(),
				ServerID: msg.ServerID,
			}

			// TODO: Add or update server
			dl.addOrUpdateServer(serverInfo)
		}
	}
}

// setupMulticastConnection sets up the UDP multicast connection
func (dl *DiscoveryListener) setupMulticastConnection(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", dl.config.MulticastAddress)
	if err != nil {
		return err
	}

	log.Printf("[DiscoveryListener] Attempting to listen on multicast address: %s", dl.config.MulticastAddress)

	// Try to find the interface that can reach the server
	var iface *net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Printf("[DiscoveryListener] Failed to get interfaces: %v", err)
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
				ip := ipNet.IP
				// Check if this interface has an IP in the 192.168.1.x range
				if ip.To4() != nil && ip.IsPrivate() && ip.String()[:10] == "192.168.1." {
					iface = &i
					log.Printf("[DiscoveryListener] Found matching interface: %s (%s)", i.Name, ip.String())
					break
				}
			}
			if iface != nil {
				break
			}
		}
	}

	// First try listening on the specific interface we found
	var conn *net.UDPConn
	if iface != nil {
		conn, err = net.ListenMulticastUDP("udp", iface, addr)
		if err != nil {
			log.Printf("[DiscoveryListener] Failed to listen on interface %s: %v", iface.Name, err)
		} else {
			log.Printf("[DiscoveryListener] Successfully listening on interface %s", iface.Name)
		}
	}

	// If that failed or no interface was found, try with nil interface
	if conn == nil {
		log.Printf("[DiscoveryListener] Trying with default interface")
		conn, err = net.ListenMulticastUDP("udp", nil, addr)
		if err != nil {
			return fmt.Errorf("failed to listen on multicast: %w", err)
		}
		log.Printf("[DiscoveryListener] Using default interface")
	}

	// Also listen on localhost for unicast messages
	localAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1901")
	if err != nil {
		log.Printf("[DiscoveryListener] Failed to resolve localhost:1901: %v", err)
	} else {
		localConn, err := net.ListenUDP("udp", localAddr)
		if err != nil {
			log.Printf("[DiscoveryListener] Failed to listen on localhost:1901: %v", err)
		} else {
			log.Printf("[DiscoveryListener] Also listening on localhost:1901")
			dl.localConn = localConn
			// Start a goroutine to handle localhost packets
			go dl.handleLocalConnection(ctx, localConn)
		}
	}

	// Log connection details
	log.Printf("[DiscoveryListener] Listening on %s (multicast: %s)", conn.LocalAddr(), dl.config.MulticastAddress)

	// Set read buffer size
	if err := conn.SetReadBuffer(2048); err != nil {
		log.Printf("[DiscoveryListener] Failed to set read buffer: %v", err)
	}

	dl.conn = conn
	return nil
}

// addOrUpdateServer adds or updates a discovered server
func (dl *DiscoveryListener) addOrUpdateServer(info *types.ServerInfo) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	key := info.ServerID
	existing, found := dl.discovered[key]

	if !found {
		// New server discovered
		dl.discovered[key] = info
		log.Printf("[DiscoveryListener] Discovered new server: %s (%s:%d)", info.Name, info.Host, info.Port)
		if dl.onServerFound != nil {
			dl.onServerFound(info)
		}
	} else {
		// Update existing server info
		existing.LastSeen = info.LastSeen
		existing.Name = info.Name
		existing.Host = info.Host
		existing.Port = info.Port
		existing.Version = info.Version
	}
}

// handleLocalConnection handles packets from the localhost unicast connection
func (dl *DiscoveryListener) handleLocalConnection(ctx context.Context, conn *net.UDPConn) {
	buffer := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[DiscoveryListener] Local connection stopping due to context cancellation")
			if err := conn.Close(); err != nil {
				log.Printf("[DiscoveryListener] Error closing local connection: %v", err)
			}
			return
		default:
			// Set read deadline to allow context cancellation
			if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				log.Printf("[DiscoveryListener] Failed to set read deadline on local connection: %v", err)
				continue
			}

			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is expected, continue listening
				}
				log.Printf("[DiscoveryListener] Local connection error: %v", err)
				return
			}

			log.Printf("[DiscoveryListener] Received %d bytes from localhost %s", n, addr)
			log.Printf("[DiscoveryListener] Raw data: %q", string(buffer[:n]))

			// Try to parse as JSON discovery message
			var msg types.DiscoveryMessage
			if err := json.Unmarshal(buffer[:n], &msg); err != nil {
				log.Printf("[DiscoveryListener] Failed to parse localhost message from %s as JSON: %v", addr, err)
				continue
			}

			// Validate that this is a BizShuffle server message
			if msg.Type != "bizshuffle_server" {
				log.Printf("[DiscoveryListener] Ignoring non-BizShuffle localhost message from %s (type: %s)", addr, msg.Type)
				continue
			}

			log.Printf("[DiscoveryListener] Received BizShuffle server message from localhost %s: server=%s (%s:%d)", addr, msg.ServerName, msg.Host, msg.Port)

			// Validate message
			if !msg.IsValid() {
				continue
			}

			// Create server info
			serverInfo := &types.ServerInfo{
				Name:     msg.ServerName,
				Host:     msg.Host,
				Port:     msg.Port,
				Version:  msg.Version,
				LastSeen: time.Now(),
				ServerID: msg.ServerID,
			}

			// Add or update server
			dl.addOrUpdateServer(serverInfo)
		}
	}
}
