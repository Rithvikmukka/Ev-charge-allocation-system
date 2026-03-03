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

type Phase3Config struct {
	ReplicationFactorN int
	WriteQuorumW       int
	ReadQuorumR        int
}

type ReserveRequest struct {
	Slot    string `json:"slot"`
	Vehicle string `json:"vehicle"`
}

type ReleaseRequest struct {
	Slot    string `json:"slot"`
	Vehicle string `json:"vehicle"`
}

type ApplyRequest struct {
	SlotKey string `json:"slotKey"`
	Slot    Slot   `json:"slot"`
}

type ApplyResponse struct {
	Applied bool   `json:"applied"`
	Message string `json:"message"`
}

type SlotResponse struct {
	SlotKey string `json:"slotKey"`
	Slot    Slot   `json:"slot"`
}

type quorumReadResult struct {
	addr string
	slot Slot
}

func (n *Node) StartHTTP(p3 Phase3Config) error {
	if p3.ReplicationFactorN <= 0 {
		p3.ReplicationFactorN = 3
	}
	if p3.WriteQuorumW <= 0 {
		p3.WriteQuorumW = 2
	}
	if p3.ReadQuorumR <= 0 {
		p3.ReadQuorumR = 2
	}

	h := n.httpHandler(p3)

	addr := fmt.Sprintf("%s:%d", n.Bind, n.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("[Phase3] Node%d HTTP bind=%s advertise=%s:%d\n", n.ID, addr, n.Host, n.Port)
	return srv.ListenAndServe()
}

func (n *Node) httpHandler(p3 Phase3Config) http.Handler {
	mux := http.NewServeMux()

	// Client-facing endpoints
	mux.HandleFunc("GET /slot", func(w http.ResponseWriter, r *http.Request) {
		slotKey := r.URL.Query().Get("slot")
		if slotKey == "" {
			http.Error(w, "missing query param: slot", http.StatusBadRequest)
			return
		}

		replicas := n.replicasForKey(slotKey, p3.ReplicationFactorN)
		slot, got, latestFrom := n.readQuorum(r.Context(), replicas, slotKey, p3.ReadQuorumR)
		if got < p3.ReadQuorumR {
			http.Error(w, fmt.Sprintf("read quorum not satisfied (R=%d got=%d)", p3.ReadQuorumR, got), http.StatusServiceUnavailable)
			return
		}

		fmt.Printf("[Phase4] READ slot=%s R=%d satisfied got=%d latestVersion=%d latestFrom=%s\n",
			slotKey, p3.ReadQuorumR, got, slot.Version, latestFrom)
		writeJSON(w, http.StatusOK, SlotResponse{SlotKey: slotKey, Slot: slot})
	})

	mux.HandleFunc("POST /reserve", func(w http.ResponseWriter, r *http.Request) {
		var req ReserveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Slot == "" || req.Vehicle == "" {
			http.Error(w, "slot and vehicle are required", http.StatusBadRequest)
			return
		}

		primary, ok := n.primaryForKey(req.Slot)
		if !ok {
			http.Error(w, "no primary available", http.StatusServiceUnavailable)
			return
		}
		if isSelfAddr(primary.Addr, n.Port) {
			n.handlePrimaryReserve(w, r, p3, req)
			return
		}

		// Phase 5: forward to primary for strict serialization.
		status, body, err := n.forwardJSON(r.Context(), "http://"+primary.Addr+"/internal/primary/reserve", req)
		if err != nil {
			http.Error(w, fmt.Sprintf("forward to primary failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})

	mux.HandleFunc("POST /release", func(w http.ResponseWriter, r *http.Request) {
		var req ReleaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Slot == "" || req.Vehicle == "" {
			http.Error(w, "slot and vehicle are required", http.StatusBadRequest)
			return
		}

		primary, ok := n.primaryForKey(req.Slot)
		if !ok {
			http.Error(w, "no primary available", http.StatusServiceUnavailable)
			return
		}
		if isSelfAddr(primary.Addr, n.Port) {
			n.handlePrimaryRelease(w, r, p3, req)
			return
		}

		status, body, err := n.forwardJSON(r.Context(), "http://"+primary.Addr+"/internal/primary/release", req)
		if err != nil {
			http.Error(w, fmt.Sprintf("forward to primary failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})

	// Phase 5: internal primary-only mutation endpoints (strict serialization)
	mux.HandleFunc("POST /internal/primary/reserve", func(w http.ResponseWriter, r *http.Request) {
		var req ReserveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Slot == "" || req.Vehicle == "" {
			http.Error(w, "slot and vehicle are required", http.StatusBadRequest)
			return
		}
		n.handlePrimaryReserve(w, r, p3, req)
	})

	mux.HandleFunc("POST /internal/primary/release", func(w http.ResponseWriter, r *http.Request) {
		var req ReleaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Slot == "" || req.Vehicle == "" {
			http.Error(w, "slot and vehicle are required", http.StatusBadRequest)
			return
		}
		n.handlePrimaryRelease(w, r, p3, req)
	})

	// Internal replication endpoint
	mux.HandleFunc("POST /internal/apply", func(w http.ResponseWriter, r *http.Request) {
		var req ApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.SlotKey == "" {
			http.Error(w, "slotKey required", http.StatusBadRequest)
			return
		}
		_, ok := n.Storage.Get(req.SlotKey)
		if !ok {
			http.Error(w, "unknown slot key", http.StatusNotFound)
			return
		}

		applied := n.Storage.ApplyIfNewer(req.SlotKey, req.Slot)
		msg := "ignored"
		if applied {
			msg = "applied"
		}
		writeJSON(w, http.StatusOK, ApplyResponse{Applied: applied, Message: msg})
	})

	// Internal read endpoint (used by quorum reads)
	mux.HandleFunc("GET /internal/get", func(w http.ResponseWriter, r *http.Request) {
		slotKey := r.URL.Query().Get("slot")
		if slotKey == "" {
			http.Error(w, "missing query param: slot", http.StatusBadRequest)
			return
		}
		slot, ok := n.Storage.Get(slotKey)
		if !ok {
			http.Error(w, "unknown slot key", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, SlotResponse{SlotKey: slotKey, Slot: slot})
	})

	// Phase 6: heartbeat receive endpoint
	mux.HandleFunc("POST /internal/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var msg HeartbeatMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if msg.From == "" {
			http.Error(w, "from required", http.StatusBadRequest)
			return
		}
		n.MarkSeen(msg.From, time.Now())
		w.WriteHeader(http.StatusOK)
	})

	// Phase 7: Bully election endpoints
	mux.HandleFunc("POST /internal/election", func(w http.ResponseWriter, r *http.Request) {
		var req ElectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		resp := n.HandleElectionRequest(r.Context(), req)
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /internal/election/announce", func(w http.ResponseWriter, r *http.Request) {
		var msg LeaderAnnounce
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		n.HandleLeaderAnnounce(msg)
		w.WriteHeader(http.StatusOK)
	})

	// Demo endpoint: trigger election manually from any node
	mux.HandleFunc("POST /election/start", func(w http.ResponseWriter, r *http.Request) {
		go n.StartElection(context.Background())
		w.WriteHeader(http.StatusAccepted)
	})

	// Phase 9: scaling / dynamic membership
	mux.HandleFunc("POST /internal/join", func(w http.ResponseWriter, r *http.Request) {
		var req JoinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.NodeID <= 0 || req.NodeAddr == "" {
			http.Error(w, "nodeId and nodeAddr required", http.StatusBadRequest)
			return
		}
		mu := n.HandleJoin(r.Context(), req)
		writeJSON(w, http.StatusOK, mu)
	})

	mux.HandleFunc("POST /internal/membership/update", func(w http.ResponseWriter, r *http.Request) {
		var mu MembershipUpdate
		if err := json.NewDecoder(r.Body).Decode(&mu); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		n.UpdateMembership(mu)
		fmt.Printf("[Phase9] Node%d applied membership update -> %d nodes\n", n.ID, len(mu.Members))
		w.WriteHeader(http.StatusOK)
	})

	// Debug endpoint: view current membership
	mux.HandleFunc("GET /membership", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, n.MembershipSnapshot())
	})

	// Runtime join trigger: POST /join?seed=<host:port>
	mux.HandleFunc("POST /join", func(w http.ResponseWriter, r *http.Request) {
		seed := r.URL.Query().Get("seed")
		if seed == "" {
			http.Error(w, "missing query param: seed", http.StatusBadRequest)
			return
		}
		go func() {
			_ = n.JoinCluster(context.Background(), seed)
		}()
		w.WriteHeader(http.StatusAccepted)
	})

	return mux
}

func (n *Node) primaryForKey(key string) (RingNode, bool) {
	ring := NewHashRing(n.Membership)
	return ring.PrimaryNode(key)
}

func (n *Node) handlePrimaryReserve(w http.ResponseWriter, r *http.Request, p3 Phase3Config, req ReserveRequest) {
	primary, ok := n.primaryForKey(req.Slot)
	if !ok || !isSelfAddr(primary.Addr, n.Port) {
		http.Error(w, "not primary", http.StatusBadRequest)
		return
	}

	next, reserved := n.Storage.ReserveIfFree(req.Slot, req.Vehicle)
	if !reserved {
		http.Error(w, "slot already booked", http.StatusConflict)
		return
	}

	replicas := n.replicasForKey(req.Slot, p3.ReplicationFactorN)
	fmt.Printf("[Phase5] PRIMARY RESERVE Node%d slot=%s vehicle=%s -> replicas: %s\n", n.ID, req.Slot, req.Vehicle, FormatRingNodes(replicas))
	acks := n.fanoutApply(r.Context(), replicas, ApplyRequest{SlotKey: req.Slot, Slot: next})
	fmt.Printf("[Phase5] PRIMARY RESERVE slot=%s version=%d acks=%d/%d W=%d\n", req.Slot, next.Version, acks, len(replicas), p3.WriteQuorumW)
	if acks < p3.WriteQuorumW {
		http.Error(w, fmt.Sprintf("write quorum not satisfied (W=%d acks=%d)", p3.WriteQuorumW, acks), http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, SlotResponse{SlotKey: req.Slot, Slot: next})
}

func (n *Node) handlePrimaryRelease(w http.ResponseWriter, r *http.Request, p3 Phase3Config, req ReleaseRequest) {
	primary, ok := n.primaryForKey(req.Slot)
	if !ok || !isSelfAddr(primary.Addr, n.Port) {
		http.Error(w, "not primary", http.StatusBadRequest)
		return
	}

	next, released := n.Storage.ReleaseIfBookedBy(req.Slot, req.Vehicle)
	if !released {
		http.Error(w, "release rejected", http.StatusConflict)
		return
	}

	replicas := n.replicasForKey(req.Slot, p3.ReplicationFactorN)
	fmt.Printf("[Phase5] PRIMARY RELEASE Node%d slot=%s vehicle=%s -> replicas: %s\n", n.ID, req.Slot, req.Vehicle, FormatRingNodes(replicas))
	acks := n.fanoutApply(r.Context(), replicas, ApplyRequest{SlotKey: req.Slot, Slot: next})
	fmt.Printf("[Phase5] PRIMARY RELEASE slot=%s version=%d acks=%d/%d W=%d\n", req.Slot, next.Version, acks, len(replicas), p3.WriteQuorumW)
	if acks < p3.WriteQuorumW {
		http.Error(w, fmt.Sprintf("write quorum not satisfied (W=%d acks=%d)", p3.WriteQuorumW, acks), http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, SlotResponse{SlotKey: req.Slot, Slot: next})
}

func (n *Node) forwardJSON(ctx context.Context, url string, payload any) (int, []byte, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out, nil
}

func (n *Node) replicasForKey(key string, replicationFactorN int) []RingNode {
	ring := NewHashRing(n.Membership)
	replicas, ok := ring.ReplicaNodes(key, replicationFactorN)
	if !ok {
		return []RingNode{{Addr: fmt.Sprintf("localhost:%d", n.Port), Token: 0}}
	}
	return replicas
}

func (n *Node) fanoutApply(ctx context.Context, replicas []RingNode, req ApplyRequest) int {
	// Phase 3: best-effort fanout; Phase 4 will enforce W and handle timeouts more explicitly.
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	acks := 0
	for _, rn := range replicas {
		if isSelfAddr(rn.Addr, n.Port) {
			acks++
			continue
		}

		body, _ := json.Marshal(req)
		httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+rn.Addr+"/internal/apply", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Printf("[Phase3] apply -> %s error: %v\n", rn.Addr, err)
			continue
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			acks++
			continue
		}
		fmt.Printf("[Phase3] apply -> %s status=%d\n", rn.Addr, resp.StatusCode)
	}
	return acks
}

func (n *Node) readQuorum(ctx context.Context, replicas []RingNode, slotKey string, readQuorumR int) (Slot, int, string) {
	client := &http.Client{Timeout: 1200 * time.Millisecond}

	results := make([]quorumReadResult, 0, len(replicas))
	got := 0
	latestFrom := ""
	latest := Slot{}

	for _, rn := range replicas {
		var slot Slot
		var ok bool
		if isSelfAddr(rn.Addr, n.Port) {
			slot, ok = n.Storage.Get(slotKey)
			if !ok {
				continue
			}
			got++
			results = append(results, quorumReadResult{addr: rn.Addr, slot: slot})
		} else {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+rn.Addr+"/internal/get?slot="+slotKey, nil)
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("[Phase4] get -> %s error: %v\n", rn.Addr, err)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				fmt.Printf("[Phase4] get -> %s status=%d\n", rn.Addr, resp.StatusCode)
				continue
			}
			var sr SlotResponse
			if err := json.Unmarshal(b, &sr); err != nil {
				fmt.Printf("[Phase4] get -> %s invalid json\n", rn.Addr)
				continue
			}
			slot = sr.Slot
			got++
			results = append(results, quorumReadResult{addr: rn.Addr, slot: slot})
		}

		if got == 1 || slot.Version > latest.Version {
			latest = slot
			latestFrom = rn.Addr
		}
		// Note: do not break early when got>=R.
		// We keep contacting remaining replicas so we can detect stale versions
		// and perform read-repair deterministically for small N.
	}

	// Read-repair: if we have a latest value and any responder is stale, push an apply.
	if got >= readQuorumR {
		for _, rr := range results {
			if rr.slot.Version < latest.Version {
				fmt.Printf("[Phase4] read-repair slot=%s pushing version=%d to %s (had %d)\n",
					slotKey, latest.Version, rr.addr, rr.slot.Version)
				_ = n.fanoutApply(ctx, []RingNode{{Addr: rr.addr}}, ApplyRequest{SlotKey: slotKey, Slot: latest})
			}
		}
	}

	return latest, got, latestFrom
}

func isSelfAddr(addr string, port int) bool {
	// Addresses may be "localhost:5001" or "127.0.0.1:5001" depending on environment.
	suffix := fmt.Sprintf(":%d", port)
	if len(addr) < len(suffix) {
		return false
	}
	return addr[len(addr)-len(suffix):] == suffix
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
