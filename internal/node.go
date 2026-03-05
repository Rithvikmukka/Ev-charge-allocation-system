package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type NodeConfig struct {
	ID       int
	Port     int
	Bind     string
	Host     string
	Peers    []string
	AutoJoin bool
}

type Node struct {
	ID            int
	Port          int
	Bind          string
	Host          string
	Token         uint64
	Membership    []string
	PeerIDs       map[string]int
	Peers         []string
	Storage       *Storage
	LogicalClock  int
	StartedAtUnix int64
	mu            sync.Mutex
	LastSeen      map[string]time.Time
	Failed        map[string]bool
	LeaderID      int
	ElectionOn    bool
	AutoJoin      bool
}

func NewNode(cfg NodeConfig) *Node {
	bind := cfg.Bind
	if strings.TrimSpace(bind) == "" {
		bind = "0.0.0.0"
	}
	host := cfg.Host
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}

	selfAdvertised := fmt.Sprintf("%s:%d", host, cfg.Port)
	token := hashToToken(fmt.Sprintf("node:%d:%s", cfg.ID, selfAdvertised))

	peerIDs := make(map[string]int, 1+len(cfg.Peers))
	peerIDs[selfAdvertised] = cfg.ID

	membership := make([]string, 0, 1+len(cfg.Peers))
	membership = append(membership, selfAdvertised)
	for _, p := range cfg.Peers {
		addr, id := ParsePeerSpec(p)
		if strings.TrimSpace(addr) == "" {
			continue
		}
		membership = append(membership, addr)
		if id != 0 {
			peerIDs[addr] = id
		}
	}
	membership = normalizeMembership(membership)

	return &Node{
		ID:            cfg.ID,
		Port:          cfg.Port,
		Bind:          bind,
		Host:          host,
		Token:         token,
		Membership:    membership,
		PeerIDs:       peerIDs,
		Peers:         cfg.Peers,
		Storage:       NewStorageWithDefaultSlots(),
		LogicalClock:  0,
		StartedAtUnix: time.Now().Unix(),
		LastSeen:      make(map[string]time.Time),
		Failed:        make(map[string]bool),
		LeaderID:      0,
		ElectionOn:    false,
		AutoJoin:      cfg.AutoJoin,
	}
}

func (n *Node) SelfAddr() string {
	return fmt.Sprintf("%s:%d", n.Host, n.Port)
}

func (n *Node) MarkSeen(addr string, t time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.LastSeen[addr] = t
	n.Failed[addr] = false
}

func (n *Node) MarkFailed(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Failed[addr] = true
}

func (n *Node) IsFailed(addr string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.Failed[addr]
}

func (n *Node) SnapshotLastSeen() map[string]time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make(map[string]time.Time, len(n.LastSeen))
	for k, v := range n.LastSeen {
		out[k] = v
	}
	return out
}

func (n *Node) SnapshotMembership() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.Membership))
	copy(out, n.Membership)
	return out
}

func (n *Node) SetMembership(members []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Membership = normalizeMembership(members)
}

func (n *Node) SetPeerID(addr string, id int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.PeerIDs == nil {
		n.PeerIDs = make(map[string]int)
	}
	n.PeerIDs[addr] = id
}

func (n *Node) Start() {
	fmt.Println("====================================================")
	fmt.Println("Distributed EV Charging Slot Allocation System")
	fmt.Println("Phase 1: Node Setup (decentralized identical nodes)")
	fmt.Println("Phase 2: Consistent Hashing (partitioning + replica placement)")
	fmt.Println("Phase 3: Replication (HTTP RPC fanout to replicas)")
	fmt.Println("----------------------------------------------------")
	fmt.Printf("NodeID=%d  Port=%d  Token=%d\n", n.ID, n.Port, n.Token)
	fmt.Printf("LogicalClock=%d\n", n.LogicalClock)
	fmt.Printf("Membership(%d)=%s\n", len(n.Membership), strings.Join(n.Membership, ", "))
	fmt.Printf("LocalStorage: initialized %d slot keys\n", len(n.Storage.Slots))

	// Phase 2 (Option A): ring is computed locally from the static membership list.
	const replicationFactorN = 3
	ring := NewHashRing(n.Membership)
	if ring.Size() > 0 {
		fmt.Printf("HashRing: %d nodes (sorted by token)\n", ring.Size())
		for i, rn := range ring.Nodes {
			fmt.Printf("  Ring[%d] %s  token=%d\n", i, rn.Addr, rn.Token)
		}
		fmt.Println("----------------------------------------------------")
		fmt.Printf("Key distribution (N=%d replicas per key)\n", replicationFactorN)
		keys := DefaultSlotKeys()
		for _, key := range keys {
			replicas, _ := ring.ReplicaNodes(key, replicationFactorN)
			primary := replicas[0]
			fmt.Printf("  Slot %s -> Primary %s | Replicas: %s\n", key, primary.Addr, FormatRingNodes(replicas))
		}
	}
	fmt.Println("====================================================")

	go func() {
		err := n.StartHTTP(Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 2})
		if err != nil {
			fmt.Printf("[Phase3] Node%d HTTP server exited: %v\n", n.ID, err)
		}
	}()

	// Phase 6: heartbeat failure detection
	ctx := context.Background()
	n.StartHeartbeat(ctx, DefaultHeartbeatConfig())

	// Phase 8: crash recovery (sync from peers on startup)
	go func() {
		time.Sleep(1 * time.Second)
		n.RecoverFromPeers(context.Background(), DefaultRecoveryConfig())
	}()

	// Auto-join on startup if enabled
	if n.AutoJoin && len(n.Peers) > 0 {
		go func() {
			time.Sleep(2 * time.Second)
			seed := n.Peers[0]
			fmt.Printf("[AutoJoin] Attempting to join cluster via seed %s\n", seed)
			err := n.JoinCluster(context.Background(), seed)
			if err != nil {
				fmt.Printf("[AutoJoin] Failed to join via %s: %v\n", seed, err)
			} else {
				fmt.Printf("[AutoJoin] Successfully joined cluster via %s\n", seed)
			}
		}()
	}

	// Networking and distributed protocols start in later phases.
	for {
		time.Sleep(1 * time.Hour)
	}
}

func hashToToken(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	// Use first 8 bytes for a stable uint64 token.
	var t uint64
	for i := 0; i < 8; i++ {
		t = (t << 8) | uint64(sum[i])
	}
	return t
}

func normalizeMembership(m []string) []string {
	seen := make(map[string]struct{}, len(m))
	out := make([]string, 0, len(m))
	for _, v := range m {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (n *Node) DebugTokenHex() string {
	b := make([]byte, 8)
	t := n.Token
	for i := 7; i >= 0; i-- {
		b[i] = byte(t & 0xff)
		t >>= 8
	}
	return hex.EncodeToString(b)
}
