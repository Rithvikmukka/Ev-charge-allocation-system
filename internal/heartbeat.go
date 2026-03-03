package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type HeartbeatConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

type HeartbeatMessage struct {
	From string `json:"from"`
	At   int64  `json:"at"`
}

func DefaultHeartbeatConfig() HeartbeatConfig {
	return HeartbeatConfig{Interval: 2 * time.Second, Timeout: 5 * time.Second}
}

func (n *Node) StartHeartbeat(ctx context.Context, cfg HeartbeatConfig) {
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	// Initialize last-seen baseline for peers (avoid immediate false failures).
	now := time.Now()
	for _, addr := range n.Membership {
		if isSelfAddr(addr, n.Port) {
			continue
		}
		n.MarkSeen(addr, now)
	}

	go n.heartbeatSender(ctx, cfg)
	go n.failureDetector(ctx, cfg)
}

func (n *Node) heartbeatSender(ctx context.Context, cfg HeartbeatConfig) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	client := &http.Client{Timeout: 1200 * time.Millisecond}
	self := n.SelfAddr()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg := HeartbeatMessage{From: self, At: time.Now().UnixNano()}
			b, _ := json.Marshal(msg)
			members := n.SnapshotMembership()
			for _, addr := range members {
				if isSelfAddr(addr, n.Port) {
					continue
				}
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/internal/heartbeat", bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					fmt.Printf("[Phase6] heartbeat -> %s error: %v\n", addr, err)
					continue
				}
				_ = resp.Body.Close()
			}
		}
	}
}

func (n *Node) failureDetector(ctx context.Context, cfg HeartbeatConfig) {
	// Check at a reasonable cadence; use Interval so detection feels responsive.
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			last := n.SnapshotLastSeen()
			for addr, t := range last {
				if isSelfAddr(addr, n.Port) {
					continue
				}
				if now.Sub(t) > cfg.Timeout {
					if !n.IsFailed(addr) {
						fmt.Printf("[Phase6] Heartbeat missed from %s (lastSeen=%s) -> MARK FAILED\n", addr, t.Format(time.RFC3339Nano))
						n.MarkFailed(addr)

						// Phase 7: if leader failed, start election.
						failedID := n.peerID(addr)
						if failedID != 0 && failedID == n.GetLeaderID() {
							fmt.Printf("[Phase7] Detected leader Node%d failed -> starting election\n", failedID)
							go n.StartElection(context.Background())
						}
					}
				}
			}
		}
	}
}
