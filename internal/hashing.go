package internal

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

type RingNode struct {
	Addr  string
	Token uint64
}

type HashRing struct {
	Nodes []RingNode
}

func NewHashRing(membership []string) HashRing {
	nodes := make([]RingNode, 0, len(membership))
	for _, addr := range membership {
		nodes = append(nodes, RingNode{Addr: addr, Token: tokenForString("member:" + addr)})
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Token == nodes[j].Token {
			return nodes[i].Addr < nodes[j].Addr
		}
		return nodes[i].Token < nodes[j].Token
	})

	return HashRing{Nodes: nodes}
}

func (r HashRing) Size() int {
	return len(r.Nodes)
}

func (r HashRing) PrimaryNode(key string) (RingNode, bool) {
	if len(r.Nodes) == 0 {
		return RingNode{}, false
	}

	k := tokenForString("key:" + key)
	idx := sort.Search(len(r.Nodes), func(i int) bool {
		return r.Nodes[i].Token >= k
	})
	if idx == len(r.Nodes) {
		idx = 0
	}
	return r.Nodes[idx], true
}

func (r HashRing) ReplicaNodes(key string, replicationFactor int) ([]RingNode, bool) {
	if len(r.Nodes) == 0 {
		return nil, false
	}
	if replicationFactor <= 0 {
		return []RingNode{}, true
	}

	primary, ok := r.PrimaryNode(key)
	if !ok {
		return nil, false
	}

	if replicationFactor > len(r.Nodes) {
		replicationFactor = len(r.Nodes)
	}

	// Find primary index, then take next nodes in the ring (wrapping).
	start := 0
	for i := range r.Nodes {
		if r.Nodes[i].Addr == primary.Addr {
			start = i
			break
		}
	}

	res := make([]RingNode, 0, replicationFactor)
	for i := 0; i < replicationFactor; i++ {
		res = append(res, r.Nodes[(start+i)%len(r.Nodes)])
	}
	return res, true
}

func FormatRingNodes(nodes []RingNode) string {
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		parts = append(parts, fmt.Sprintf("%s", n.Addr))
	}
	return joinComma(parts)
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += ", " + parts[i]
	}
	return out
}

func tokenForString(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	var t uint64
	for i := 0; i < 8; i++ {
		t = (t << 8) | uint64(sum[i])
	}
	return t
}
