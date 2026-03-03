package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestPhase5_ConcurrentReserveOnlyOneSucceeds(t *testing.T) {
	n1 := NewNode(NodeConfig{ID: 1, Port: 1, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n2 := NewNode(NodeConfig{ID: 2, Port: 2, Host: "127.0.0.1", Bind: "127.0.0.1"})
	n3 := NewNode(NodeConfig{ID: 3, Port: 3, Host: "127.0.0.1", Bind: "127.0.0.1"})

	p := Phase3Config{ReplicationFactorN: 3, WriteQuorumW: 2, ReadQuorumR: 1}

	s1 := httptest.NewServer(n1.httpHandler(p))
	defer s1.Close()
	s2 := httptest.NewServer(n2.httpHandler(p))
	defer s2.Close()
	s3 := httptest.NewServer(n3.httpHandler(p))
	defer s3.Close()

	addr1, host1, port1 := mustHostPort(t, s1.URL)
	addr2, host2, port2 := mustHostPort(t, s2.URL)
	addr3, host3, port3 := mustHostPort(t, s3.URL)

	n1.Host, n1.Port = host1, port1
	n2.Host, n2.Port = host2, port2
	n3.Host, n3.Port = host3, port3

	membership := []string{addr1, addr2, addr3}
	n1.Membership, n2.Membership, n3.Membership = membership, membership, membership

	// Two different coordinators try to reserve the same slot concurrently.
	slotKey := "StationA-Slot3"
	bodyA, _ := json.Marshal(ReserveRequest{Slot: slotKey, Vehicle: "EV-A"})
	bodyB, _ := json.Marshal(ReserveRequest{Slot: slotKey, Vehicle: "EV-B"})

	var wg sync.WaitGroup
	wg.Add(2)

	errCh := make(chan error, 2)
	statusCh := make(chan int, 2)

	go func() {
		defer wg.Done()
		resp, err := http.Post(s2.URL+"/reserve", "application/json", bytes.NewReader(bodyA))
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		statusCh <- resp.StatusCode
	}()

	go func() {
		defer wg.Done()
		resp, err := http.Post(s3.URL+"/reserve", "application/json", bytes.NewReader(bodyB))
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		statusCh <- resp.StatusCode
	}()

	wg.Wait()
	close(errCh)
	close(statusCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("reserve request failed: %v", err)
		}
	}

	statuses := make([]int, 0, 2)
	for st := range statusCh {
		statuses = append(statuses, st)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(statuses))
	}

	okCount := 0
	conflictCount := 0
	for _, st := range statuses {
		switch st {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected status code: %d", st)
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected one success and one conflict; got ok=%d conflict=%d", okCount, conflictCount)
	}
}

func mustHostPort(t *testing.T, rawURL string) (addr, host string, port int) {
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	hostport := u.Host
	parts := strings.Split(hostport, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected hostport: %s", hostport)
	}
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return hostport, parts[0], p
}
