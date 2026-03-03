package internal

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func mustHostPortP8(t *testing.T, rawURL string) (addr, host string, port int) {
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	parts := strings.Split(u.Host, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected hostport: %s", u.Host)
	}
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return u.Host, parts[0], p
}

func TestPhase8_RecoveryUpdatesStaleNodeFromPeers(t *testing.T) {
	// n1 will be the stale/restarted node.
	n1 := NewNode(NodeConfig{ID: 1, Port: 1, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n2 := NewNode(NodeConfig{ID: 2, Port: 2, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n3 := NewNode(NodeConfig{ID: 3, Port: 3, Host: "127.0.0.1", Bind: "127.0.0.1"})

	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 2}

	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()
	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s3 := httptest.NewServer(n3.httpHandler(p))
	defer s3.Close()

	addr1, host1, port1 := mustHostPortP8(t, s1.URL)
	addr2, host2, port2 := mustHostPortP8(t, s2.URL)
	addr3, host3, port3 := mustHostPortP8(t, s3.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3

	membership := []string{addr1, addr2, addr3}
	n1.Membership, n2.Membership, n3.Membership = membership, membership, membership

	// Simulate peers having newer state.
	key := "StationB-Slot1"
	n2.Storage.ApplyIfNewer(key, Slot{Status: SlotBooked, VehicleID: "EV-REC", Version: 5})
	n3.Storage.ApplyIfNewer(key, Slot{Status: SlotBooked, VehicleID: "EV-REC", Version: 5})
	// n1 is stale.
	n1.Storage.ApplyIfNewer(key, Slot{Status: SlotBooked, VehicleID: "EV-REC", Version: 1})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	n1.RecoverFromPeers(ctx, RecoveryConfig{ReplicationFactorN: 3, ReadQuorumR: 2})

	got, _ := n1.Storage.Get(key)
	if got.Version != 5 {
		t.Fatalf("expected recovery to update n1 to version 5, got %d", got.Version)
	}
}
