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

func mustHostPortP3(t *testing.T, rawURL string) (addr, host string, port int) {
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

func TestPhase3_ReplicationFanoutAppliesToReplicas(t *testing.T) {
	// Create 3 nodes with independent storage.
	n1 := NewNode(NodeConfig{ID: 1, Port: 5001, Peers: []string{"127.0.0.1:5002", "127.0.0.1:5003"}})
	n2 := NewNode(NodeConfig{ID: 2, Port: 5002, Peers: []string{"127.0.0.1:5001", "127.0.0.1:5003"}})
	n3 := NewNode(NodeConfig{ID: 3, Port: 5003, Peers: []string{"127.0.0.1:5001", "127.0.0.1:5002"}})

	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 1, ReadQuorumR: 1}
	// Start httptest servers for replicas 2 and 3.
	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s3 := httptest.NewServer(n3.httpHandler(p))
	defer s3.Close()

	// Start httptest server for node1 and rewrite advertised identity to match actual servers.
	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()

	addr1, host1, port1 := mustHostPortP3(t, s1.URL)
	addr2, host2, port2 := mustHostPortP3(t, s2.URL)
	addr3, host3, port3 := mustHostPortP3(t, s3.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3

	// Rewrite membership addresses so fanout targets httptest servers.
	n1.Membership = []string{addr1, addr2, addr3}
	n2.Membership = n1.Membership
	n3.Membership = n1.Membership

	body, _ := json.Marshal(ReserveRequest{Slot: "StationA-Slot2", Vehicle: "EVX"})
	resp, err := http.Post(s1.URL+"/reserve", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("reserve request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("expected 2xx, got %d", resp.StatusCode)
	}

	// Verify replicas have applied.
	slot2, _ := n2.Storage.Get("StationA-Slot2")
	slot3, _ := n3.Storage.Get("StationA-Slot2")
	if slot2.Status != SlotBooked || slot2.VehicleID != "EVX" || slot2.Version != 1 {
		t.Fatalf("replica2 not updated: %+v", slot2)
	}
	if slot3.Status != SlotBooked || slot3.VehicleID != "EVX" || slot3.Version != 1 {
		t.Fatalf("replica3 not updated: %+v", slot3)
	}
}
