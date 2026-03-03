package internal

import "testing"

func TestPhase2_HashRingDeterministicPrimaryAndReplicas(t *testing.T) {
	membership := []string{"localhost:5001", "localhost:5002", "localhost:5003"}
	ring1 := NewHashRing(membership)
	ring2 := NewHashRing(membership)

	key := "StationA-Slot1"
	p1, ok1 := ring1.PrimaryNode(key)
	p2, ok2 := ring2.PrimaryNode(key)
	if !ok1 || !ok2 {
		t.Fatal("expected primary node")
	}
	if p1.Addr != p2.Addr {
		t.Fatalf("expected deterministic primary, got %s vs %s", p1.Addr, p2.Addr)
	}

	reps, ok := ring1.ReplicaNodes(key, 3)
	if !ok {
		t.Fatal("expected replicas")
	}
	if len(reps) != 3 {
		t.Fatalf("expected 3 replicas, got %d", len(reps))
	}
	if reps[0].Addr != p1.Addr {
		t.Fatalf("expected replica[0] to be primary %s, got %s", p1.Addr, reps[0].Addr)
	}

	reps2, _ := ring1.ReplicaNodes(key, 10)
	if len(reps2) != len(membership) {
		t.Fatalf("expected replicationFactor capped to membership size=%d, got %d", len(membership), len(reps2))
	}
}
