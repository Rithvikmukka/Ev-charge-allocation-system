package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func mustHostPortP4(t *testing.T, rawURL string) (addr, host string, port int) {
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

func TestPhase4_ReadQuorumReturnsLatestAndRepairsStaleReplica(t *testing.T) {
	n1 := NewNode(NodeConfig{ID: 1, Port: 5001, Peers: []string{}})
	n2 := NewNode(NodeConfig{ID: 2, Port: 5002, Peers: []string{}})
	n3 := NewNode(NodeConfig{ID: 3, Port: 5003, Peers: []string{}})

	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 2}

	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s3 := httptest.NewServer(n3.httpHandler(p))
	defer s3.Close()
	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()

	addr1, host1, port1 := mustHostPortP4(t, s1.URL)
	addr2, host2, port2 := mustHostPortP4(t, s2.URL)
	addr3, host3, port3 := mustHostPortP4(t, s3.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3

	membership := []string{addr1, addr2, addr3}
	n1.Membership = membership
	n2.Membership = membership
	n3.Membership = membership

	// Create divergence: node2 has version 2, node3 is stale at version 1.
	n2.Storage.ApplyIfNewer("StationC-Slot1", Slot{Status: SlotBooked, VehicleID: "EV9", Version: 2})
	n3.Storage.ApplyIfNewer("StationC-Slot1", Slot{Status: SlotBooked, VehicleID: "EV9", Version: 1})
	n1.Storage.ApplyIfNewer("StationC-Slot1", Slot{Status: SlotBooked, VehicleID: "EV9", Version: 2})

	// Read from node1: should use quorum read and then repair node3 to version 2.
	resp, err := http.Get(s1.URL + "/slot?slot=StationC-Slot1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	repaired, _ := n3.Storage.Get("StationC-Slot1")
	if repaired.Version != 2 {
		t.Fatalf("expected read-repair to update replica3 to version 2, got %d", repaired.Version)
	}
}

func TestPhase4_WriteQuorumFailsIfNotEnoughAcks(t *testing.T) {
	n1 := NewNode(NodeConfig{ID: 1, Port: 5001, Peers: []string{}})
	n2 := NewNode(NodeConfig{ID: 2, Port: 5002, Peers: []string{}})

	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 1}

	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()

	addr1, host1, port1 := mustHostPortP4(t, s1.URL)
	addr2, host2, port2 := mustHostPortP4(t, s2.URL)
	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2

	// Membership includes only 2 nodes (N will cap to 2), but W=2 should still be satisfiable.
	membership := []string{addr1, addr2}
	n1.Membership = membership
	n2.Membership = membership

	body, _ := json.Marshal(ReserveRequest{Slot: "StationB-Slot2", Vehicle: "EVQ"})
	resp, err := http.Post(s1.URL+"/reserve", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("reserve request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Now make W impossible by setting W=2 but only one node in membership.
	n1.Membership = []string{addr1}
	body2, _ := json.Marshal(ReserveRequest{Slot: "StationB-Slot3", Vehicle: "EVQ"})
	resp2, err := http.Post(s1.URL+"/reserve", "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("reserve request 2 failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 200 {
		t.Fatalf("expected non-200 when W cannot be satisfied")
	}
}
