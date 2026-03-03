package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParsePeerSpec supports:
// - "host:port" (id unknown => 0)
// - "<id>@host:port" (preferred for Bully election)
func ParsePeerSpec(s string) (addr string, id int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}
	if !strings.Contains(s, "@") {
		return s, 0
	}

	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 {
		return s, 0
	}
	id64, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return strings.TrimSpace(parts[1]), 0
	}
	return strings.TrimSpace(parts[1]), int(id64)
}

type ElectionRequest struct {
	FromID   int    `json:"fromId"`
	FromAddr string `json:"fromAddr"`
}

type ElectionResponse struct {
	OK bool `json:"ok"`
}

type LeaderAnnounce struct {
	LeaderID   int    `json:"leaderId"`
	LeaderAddr string `json:"leaderAddr"`
}

func (n *Node) GetLeaderID() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.LeaderID
}

func (n *Node) setLeader(id int) {
	n.mu.Lock()
	n.LeaderID = id
	n.ElectionOn = false
	n.mu.Unlock()
}

func (n *Node) setElectionOn(on bool) {
	n.mu.Lock()
	n.ElectionOn = on
	n.mu.Unlock()
}

func (n *Node) peerID(addr string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.PeerIDs[addr]
}

// StartElection triggers Bully election. Highest node ID wins.
func (n *Node) StartElection(ctx context.Context) {
	// Avoid repeated elections.
	n.mu.Lock()
	if n.ElectionOn {
		n.mu.Unlock()
		return
	}
	n.ElectionOn = true
	n.mu.Unlock()

	selfID := n.ID
	selfAddr := n.SelfAddr()

	// Contact higher-ID nodes.
	higher := make([]string, 0)
	for _, addr := range n.Membership {
		pid := n.peerID(addr)
		if pid > selfID {
			higher = append(higher, addr)
		}
	}

	okReceived := false
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	for _, addr := range higher {
		reqBody, _ := json.Marshal(ElectionRequest{FromID: selfID, FromAddr: selfAddr})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/internal/election", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var er ElectionResponse
			_ = json.Unmarshal(b, &er)
			if er.OK {
				okReceived = true
			}
		}
	}

	if !okReceived {
		// No higher node responded => I am leader.
		n.setLeader(selfID)
		fmt.Printf("[Phase7] Node%d becomes LEADER (no higher OK)\n", selfID)
		n.announceLeader(ctx, selfID, selfAddr)
		return
	}

	// Higher node exists; wait for leader announcement.
	// Keep ElectionOn true until leader announce arrives.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			// If nobody announced, retry.
			n.setElectionOn(false)
			n.StartElection(context.Background())
		}
	}()
}

func (n *Node) HandleElectionRequest(ctx context.Context, req ElectionRequest) ElectionResponse {
	if req.FromAddr != "" {
		n.MarkSeen(req.FromAddr, time.Now())
	}
	// If sender has lower ID, respond OK and start own election.
	if req.FromID < n.ID {
		fmt.Printf("[Phase7] Node%d received ELECTION from Node%d -> OK and start election\n", n.ID, req.FromID)
		go n.StartElection(context.Background())
		return ElectionResponse{OK: true}
	}
	return ElectionResponse{OK: false}
}

func (n *Node) HandleLeaderAnnounce(msg LeaderAnnounce) {
	if msg.LeaderID <= 0 {
		return
	}
	n.setLeader(msg.LeaderID)
	fmt.Printf("[Phase7] Node%d sets leader=Node%d\n", n.ID, msg.LeaderID)
}

func (n *Node) announceLeader(ctx context.Context, leaderID int, leaderAddr string) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	body, _ := json.Marshal(LeaderAnnounce{LeaderID: leaderID, LeaderAddr: leaderAddr})
	for _, addr := range n.Membership {
		if isSelfAddr(addr, n.Port) {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/internal/election/announce", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
	}
}
