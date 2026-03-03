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

func mustHostPortP9(t *testing.T, rawURL string) (addr, host string, port int) {
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

func TestPhase9_JoinAugmentsMembershipAndRebalances(t *testing.T) {
	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 2}

	n1 := NewNode(NodeConfig{ID: 1, Port: 1, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n2 := NewNode(NodeConfig{ID: 2, Port: 2, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n3 := NewNode(NodeConfig{ID: 3, Port: 3, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n4 := NewNode(NodeConfig{ID: 4, Port: 4, Host: "127.0.0.1", Bind: "127.0.0.1"})

	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()
	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s3 := httptest.NewServer(n3.httpHandler(p))
	defer s3.Close()
	s4 := httptest.NewServer(n4.httpHandler(p))
	defer s4.Close()

	addr1, host1, port1 := mustHostPortP9(t, s1.URL)
	addr2, host2, port2 := mustHostPortP9(t, s2.URL)
	addr3, host3, port3 := mustHostPortP9(t, s3.URL)
	addr4, host4, port4 := mustHostPortP9(t, s4.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3
	n4.Host, n4.Port = host4, port4

	m3 := []string{addr1, addr2, addr3}
	n1.Membership, n2.Membership, n3.Membership = m3, m3, m3

	// Provide known peer IDs for election + failure detection modules.
	for _, n := range []*Node{n1, n2, n3} {
		n.PeerIDs[addr1] = 1
		n.PeerIDs[addr2] = 2
		n.PeerIDs[addr3] = 3
	}

	// Find a key that will include n4 in its replica set after join, but not before.
	var key string
	before := NewHashRing(m3)
	after := NewHashRing([]string{addr1, addr2, addr3, addr4})
	for _, k := range DefaultSlotKeys() {
		rb, _ := before.ReplicaNodes(k, 3)
		ra, _ := after.ReplicaNodes(k, 3)
		inBefore := false
		for _, rn := range rb {
			if rn.Addr == addr4 {
				inBefore = true
				break
			}
		}
		inAfter := false
		for _, rn := range ra {
			if rn.Addr == addr4 {
				inAfter = true
				break
			}
		}
		if !inBefore && inAfter {
			key = k
			break
		}
	}
	if key == "" {
		t.Fatalf("could not find a key that moves to include the joining node")
	}

	// Seed newer state on existing nodes; n4 is stale.
	n2.Storage.ApplyIfNewer(key, Slot{Status: SlotBooked, VehicleID: "EV-SCALE", Version: 7})
	n3.Storage.ApplyIfNewer(key, Slot{Status: SlotBooked, VehicleID: "EV-SCALE", Version: 7})
	n4.Storage.ApplyIfNewer(key, Slot{Status: SlotFree, VehicleID: "", Version: 0})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// n4 joins using seed n1.
	// After join, seed broadcasts membership to everyone and n4 runs recovery/rebalance.
	n4.Membership = []string{addr4, addr1}
	n4.PeerIDs[addr4] = 4
	n4.PeerIDs[addr1] = 1
	if err := n4.JoinCluster(ctx, addr1); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	// Allow broadcast updates to be applied.
	time.Sleep(200 * time.Millisecond)

	mu1 := n1.MembershipSnapshot()
	mu2 := n2.MembershipSnapshot()
	mu3 := n3.MembershipSnapshot()
	mu4 := n4.MembershipSnapshot()

	if len(mu1.Members) != 4 || len(mu2.Members) != 4 || len(mu3.Members) != 4 || len(mu4.Members) != 4 {
		t.Fatalf("expected 4-node membership everywhere; got n1=%d n2=%d n3=%d n4=%d", len(mu1.Members), len(mu2.Members), len(mu3.Members), len(mu4.Members))
	}

	// Rebalance expectation: n4 should have recovered the moved key to version 7.
	got, _ := n4.Storage.Get(key)
	if got.Version != 7 {
		t.Fatalf("expected joining node to recover moved key %s to version 7, got %d", key, got.Version)
	}
}
