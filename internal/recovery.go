package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RecoveryConfig struct {
	ReplicationFactorN int
	ReadQuorumR        int
}

func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{ReplicationFactorN: 3, ReadQuorumR: 2}
}

// RecoverFromPeers performs crash recovery by syncing keys this node is responsible
// for (i.e., keys for which this node is in the replica set).
//
// Distributed concept: state transfer / replica recovery.
func (n *Node) RecoverFromPeers(ctx context.Context, cfg RecoveryConfig) {
	if cfg.ReplicationFactorN <= 0 {
		cfg.ReplicationFactorN = 3
	}
	if cfg.ReadQuorumR <= 0 {
		cfg.ReadQuorumR = 2
	}

	client := &http.Client{Timeout: 1500 * time.Millisecond}
	keys := DefaultSlotKeys()
	self := n.SelfAddr()

	ring := NewHashRing(n.Membership)
	for _, key := range keys {
		replicas, ok := ring.ReplicaNodes(key, cfg.ReplicationFactorN)
		if !ok {
			continue
		}

		inReplicaSet := false
		for _, rn := range replicas {
			if rn.Addr == self || isSelfAddr(rn.Addr, n.Port) {
				inReplicaSet = true
				break
			}
		}
		if !inReplicaSet {
			continue
		}

		local, _ := n.Storage.Get(key)
		latest := local
		got := 0

		for _, rn := range replicas {
			// skip self
			if rn.Addr == self || isSelfAddr(rn.Addr, n.Port) {
				got++
				continue
			}

			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+rn.Addr+"/internal/get?slot="+key, nil)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				continue
			}
			var sr SlotResponse
			if err := json.Unmarshal(b, &sr); err != nil {
				continue
			}
			got++
			if sr.Slot.Version > latest.Version {
				latest = sr.Slot
			}
		}

		if got < cfg.ReadQuorumR {
			fmt.Printf("[Phase8] Recovery: slot=%s skipped (R=%d got=%d)\n", key, cfg.ReadQuorumR, got)
			continue
		}

		if latest.Version > local.Version {
			n.Storage.ApplyIfNewer(key, latest)
			fmt.Printf("[Phase8] Recovery: slot=%s updated localVersion=%d -> %d\n", key, local.Version, latest.Version)
		}
	}
}
