package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type JoinRequest struct {
	NodeID   int    `json:"nodeId"`
	NodeAddr string `json:"nodeAddr"`
}

type MembershipUpdate struct {
	Members []string       `json:"members"`
	PeerIDs map[string]int `json:"peerIds"`
}

func (n *Node) UpdateMembership(mu MembershipUpdate) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(mu.Members) > 0 {
		n.Membership = normalizeMembership(mu.Members)
	}
	if n.PeerIDs == nil {
		n.PeerIDs = make(map[string]int)
	}
	for addr, id := range mu.PeerIDs {
		if id != 0 {
			n.PeerIDs[addr] = id
		}
	}

	// Ensure we always have our own ID mapping.
	self := n.SelfAddr()
	if n.ID != 0 {
		n.PeerIDs[self] = n.ID
	}
}

func (n *Node) MembershipSnapshot() MembershipUpdate {
	n.mu.Lock()
	defer n.mu.Unlock()

	members := make([]string, len(n.Membership))
	copy(members, n.Membership)

	peerIDs := make(map[string]int, len(n.PeerIDs))
	for k, v := range n.PeerIDs {
		peerIDs[k] = v
	}
	return MembershipUpdate{Members: members, PeerIDs: peerIDs}
}

// HandleJoin is invoked on a seed/existing node.
// It adds the new node to membership and broadcasts to all known members.
func (n *Node) HandleJoin(ctx context.Context, req JoinRequest) MembershipUpdate {
	cur := n.MembershipSnapshot()
	members := append(cur.Members, req.NodeAddr)
	cur.Members = normalizeMembership(members)
	if cur.PeerIDs == nil {
		cur.PeerIDs = make(map[string]int)
	}
	if req.NodeID != 0 {
		cur.PeerIDs[req.NodeAddr] = req.NodeID
	}

	n.UpdateMembership(cur)

	fmt.Printf("[Phase9] Node%d accepted join: Node%d %s -> membership now %d nodes\n", n.ID, req.NodeID, req.NodeAddr, len(cur.Members))
	go n.BroadcastMembership(context.Background(), cur)
	return cur
}

func (n *Node) BroadcastMembership(ctx context.Context, mu MembershipUpdate) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	b, _ := json.Marshal(mu)

	for _, addr := range mu.Members {
		if isSelfAddr(addr, n.Port) {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/internal/membership/update", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
	}
}

// JoinCluster contacts a seed peer, obtains the latest membership, installs it locally,
// and triggers a recovery/rebalance pass.
func (n *Node) JoinCluster(ctx context.Context, seedAddr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	payload, _ := json.Marshal(JoinRequest{NodeID: n.ID, NodeAddr: n.SelfAddr()})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+seedAddr+"/internal/join", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("seed returned status %d", resp.StatusCode)
	}

	var mu MembershipUpdate
	if err := json.Unmarshal(b, &mu); err != nil {
		return err
	}

	n.UpdateMembership(mu)
	fmt.Printf("[Phase9] Node%d joined via %s -> membership now %d nodes\n", n.ID, seedAddr, len(mu.Members))

	// Rebalance: pull any keys we are now responsible for.
	n.RecoverFromPeers(ctx, DefaultRecoveryConfig())
	return nil
}
