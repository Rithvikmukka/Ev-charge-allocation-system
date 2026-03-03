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

func mustHostPortP7(t *testing.T, rawURL string) (addr, host string, port int) {
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

func TestPhase7_BullyElectionHighestIDWins(t *testing.T) {
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

	addr1, host1, port1 := mustHostPortP7(t, s1.URL)
	addr2, host2, port2 := mustHostPortP7(t, s2.URL)
	addr3, host3, port3 := mustHostPortP7(t, s3.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3

	membership := []string{addr1, addr2, addr3}
	n1.Membership, n2.Membership, n3.Membership = membership, membership, membership

	// Provide peer IDs for bully election.
	n1.PeerIDs[addr1] = 1
	n1.PeerIDs[addr2] = 2
	n1.PeerIDs[addr3] = 3
	n2.PeerIDs[addr1] = 1
	n2.PeerIDs[addr2] = 2
	n2.PeerIDs[addr3] = 3
	n3.PeerIDs[addr1] = 1
	n3.PeerIDs[addr2] = 2
	n3.PeerIDs[addr3] = 3

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	n1.StartElection(ctx)
	time.Sleep(200 * time.Millisecond)

	if n1.GetLeaderID() != 3 {
		t.Fatalf("expected leader=3 on n1, got %d", n1.GetLeaderID())
	}
	if n2.GetLeaderID() != 3 {
		t.Fatalf("expected leader=3 on n2, got %d", n2.GetLeaderID())
	}
	if n3.GetLeaderID() != 3 {
		t.Fatalf("expected leader=3 on n3, got %d", n3.GetLeaderID())
	}
}
