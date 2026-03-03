package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPhase6_HeartbeatMarksSeenAndDetectsFailure(t *testing.T) {
	n1 := NewNode(NodeConfig{ID: 1, Port: 5001, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n2 := NewNode(NodeConfig{ID: 2, Port: 5002, Host: "127.0.0.1", Bind: "127.0.0.1"})

	p := Phase3Config{ReplicationFactorN: 2, WriteQuorumW: 1, ReadQuorumR: 1}

	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()

	// n1 will heartbeat to n2.
	n1.SetMembership([]string{
		"127.0.0.1:5001",
		strings.TrimPrefix(s2.URL, "http://"),
	})
	n2.SetMembership(n1.SnapshotMembership())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := HeartbeatConfig{Interval: 20 * time.Millisecond, Timeout: 80 * time.Millisecond}
	n1.StartHeartbeat(ctx, cfg)

	// Wait for at least one heartbeat to be delivered.
	time.Sleep(60 * time.Millisecond)
	if n2.IsFailed(n1.SelfAddr()) {
		t.Fatalf("expected n1 not failed on n2 after heartbeats")
	}

	// Stop sending heartbeats and ensure failure is detected.
	cancel()
	// Let detector on n2 run by starting its detector alone.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	n2.StartHeartbeat(ctx2, cfg)

	// Simulate no incoming heartbeat by ensuring no sender targets n2.
	n2.SetMembership([]string{strings.TrimPrefix(s2.URL, "http://")})

	time.Sleep(200 * time.Millisecond)
	// n2 failure detector should mark n1 failed based on lastSeen map.
	// Note: lastSeen was updated earlier when heartbeat was received.
	if !n2.IsFailed(n1.SelfAddr()) {
		// Allow one more interval in case of scheduling.
		time.Sleep(100 * time.Millisecond)
	}
	if !n2.IsFailed(n1.SelfAddr()) {
		// As a final sanity check, ensure endpoint is reachable (test infra).
		resp, err := http.Get(s2.URL + "/internal/get?slot=StationA-Slot1")
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("expected n1 to be marked failed on n2 after timeout")
	}
}
