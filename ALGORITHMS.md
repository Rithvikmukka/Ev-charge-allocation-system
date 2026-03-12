# Distributed Algorithms in EV Charging System

This document explains all the distributed algorithms implemented in the EV charging slot allocation system and why each algorithm is essential for building a robust, fault-tolerant distributed system.

## Table of Contents

1. [Consistent Hashing](#consistent-hashing)
2. [Quorum Replication](#quorum-replication)
3. [Primary Serialization](#primary-serialization)
4. [Heartbeat Failure Detection](#heartbeat-failure-detection)
5. [Bully Leader Election](#bully-leader-election)
6. [Crash Recovery & State Transfer](#crash-recovery--state-transfer)
7. [Dynamic Membership & Rebalancing](#dynamic-membership--rebalancing)

---

## Consistent Hashing

### What It Is
Consistent hashing is a special hashing technique that minimizes key remapping when the number of nodes in a distributed system changes. Unlike traditional modulo hashing, it ensures that only a small fraction of keys need to be moved when nodes are added or removed.

### Implementation
- **File**: `internal/hashring.go`
- **Hash Function**: Uses a ring-based approach where both nodes and data keys are placed on a virtual ring
- **Replica Selection**: Each key is assigned to N=3 replica nodes in clockwise direction

### Why It's Used

#### 1. **Load Distribution**
```go
// Evenly distributes charging slots across all nodes
slot := "StationA-Slot1"
replicas := hashring.GetReplicas(slot, 3) // Returns 3 different nodes
```

#### 2. **Minimal Rebalancing**
When a new charging station node joins:
- **Traditional hashing**: 100% of keys would need remapping
- **Consistent hashing**: Only ~1/N keys move (N = total nodes)

#### 3. **Fault Tolerance**
If a node fails, only its keys need to be redistributed to adjacent nodes on the ring, minimizing the impact on the system.

### Example Scenario
```
Ring: [Node1] -> [Node2] -> [Node3] -> [Node1]
Keys:  "StationA-Slot1" -> Node1, Node2, Node3
       "StationB-Slot1" -> Node2, Node3, Node1
       "StationC-Slot1" -> Node3, Node1, Node2
```

---

## Quorum Replication

### What It Is
Quorum replication ensures data consistency by requiring multiple nodes to agree on read and write operations. It uses the parameters N, W, and R:
- **N**: Number of replicas (3)
- **W**: Write quorum (2) - nodes that must acknowledge a write
- **R**: Read quorum (2) - nodes that must respond to a read

### Implementation
- **File**: `internal/replication.go`
- **Configuration**: N=3, W=2, R=2
- **Consistency Model**: Eventually consistent with strong guarantees

### Why It's Used

#### 1. **Data Durability**
```go
// Write succeeds only if 2 out of 3 replicas acknowledge
if writeQuorumSatisfied(acknowledgments, 2) {
    return SUCCESS
}
```

#### 2. **Fault Tolerance**
The system can tolerate the failure of up to `N - W = 1` node for writes and `N - R = 1` node for reads.

#### 3. **Consistency Guarantees**
With `W + R > N` (2 + 2 > 3), every read is guaranteed to see the most recent write, preventing stale data.

### Example Flow
```
Reserve "StationA-Slot1":
1. Coordinator identifies replicas: Node1, Node2, Node3
2. Sends write request to all 3 replicas
3. Waits for W=2 acknowledgments
4. Returns success once 2 replicas confirm
```

---

## Primary Serialization

### What It Is
Primary serialization ensures that all operations on a specific data item are processed sequentially through a designated primary node, preventing race conditions and double booking.

### Implementation
- **Mechanism**: The first replica in the consistent hash ring acts as the primary
- **Protocol**: All write operations for a slot must go through its primary node
- **Conflict Resolution**: Primary rejects conflicting operations

### Why It's Used

#### 1. **Prevent Double Booking**
```go
// Primary node checks slot state before allowing reservation
if slot.State == BOOKED {
    return CONFLICT // Prevents double booking
}
```

#### 2. **Deterministic Ordering**
All operations for a slot are processed in the order they arrive at the primary, ensuring a single, consistent state.

#### 3. **Simplified Conflict Resolution**
Instead of complex distributed locking, we use a simple primary-based approach.

### Example Scenario
```
Two users try to book "StationA-Slot1" simultaneously:
1. Both requests hit Node1 (primary for this slot)
2. Node1 processes requests sequentially
3. First request succeeds, second gets CONFLICT
4. Result: No double booking guaranteed
```

---

## Heartbeat Failure Detection

### What It Is
Heartbeat failure detection is a mechanism where nodes periodically exchange "heartbeat" messages to detect when a node has crashed or become unreachable.

### Implementation
- **File**: `internal/heartbeat.go`
- **Interval**: Nodes send heartbeats every 2 seconds
- **Timeout**: Node is marked failed after 5 missed heartbeats (10 seconds)
- **Gossip Protocol**: Failed nodes are propagated through the cluster

### Why It's Used

#### 1. **Quick Failure Detection**
```go
if time.Since(lastHeartbeat) > timeout {
    markNodeAsFailed(nodeID)
    triggerRecoveryIfNeeded()
}
```

#### 2. **Cluster Health Monitoring**
The system maintains a real-time view of which nodes are alive and which have failed.

#### 3. **Trigger for Recovery**
Heartbeat failures automatically trigger recovery processes like leader election and data rebalancing.

### Example Flow
```
Normal Operation:
Node1 -> Node2: heartbeat (t=0)
Node1 -> Node3: heartbeat (t=0)
Node1 -> Node2: heartbeat (t=2)
Node1 -> Node3: heartbeat (t=2)

Failure Detection:
Node3 crashes at t=3
Node1 misses heartbeat at t=4,6,8,10,12
At t=12: Node1 marks Node3 as FAILED
```

---

## Bully Leader Election

### What It Is
The bully algorithm is a simple leader election protocol where the node with the highest ID among alive nodes becomes the leader. "Bully" because higher ID nodes can "bully" lower ID nodes to become leader.

### Implementation
- **File**: `internal/election.go`
- **Trigger**: Leader failure detected via heartbeat
- **Protocol**: Nodes with higher IDs can take over leadership

### Why It's Used

#### 1. **Simple and Deterministic**
```go
if nodeID > currentLeaderID {
    startElection() // Higher ID can take over
}
```

#### 2. **Fast Convergence**
Election completes in O(log N) message complexity, typically within seconds.

#### 3. **No Split Brain**
The highest ID rule ensures exactly one leader at any time.

### Example Election
```
Initial State: Node3 (ID=3) is leader
Node3 crashes -> Heartbeat timeout detected

Election Process:
1. Node1 and Node2 detect leader failure
2. Node2 (ID=2) sends ELECTION to Node1 (ID=1)
3. Node1 responds OK (lower ID)
4. Node2 becomes new leader (highest alive ID)
5. Node2 broadcasts COORDINATOR message
```

---

## Crash Recovery & State Transfer

### What It Is
Crash recovery allows a failed node to rejoin the cluster and synchronize its state with the current cluster state, ensuring it doesn't serve stale data.

### Implementation
- **File**: `internal/recovery.go`
- **Process**: Node pulls latest state for keys it's responsible for
- **State Transfer**: Uses anti-entropy to reconcile differences

### Why It's Used

#### 1. **Data Consistency**
```go
for eachSlotIResponsibleFor {
    latestState = queryReplicas(slot)
    updateLocalState(latestState)
}
```

#### 2. **Seamless Reintegration**
Recovered nodes automatically catch up without manual intervention.

#### 3. **Prevents Stale Reads**
Ensures restarted nodes don't serve outdated booking information.

### Example Recovery
```
Node3 crashes and restarts:
1. Node3 joins cluster with old state
2. Node3 identifies slots it should own
3. Node3 queries current replicas for each slot
4. Node3 updates local state with latest versions
5. Node3 is ready to serve requests
```

---

## Dynamic Membership & Rebalancing

### What It Is
Dynamic membership allows nodes to join and leave the cluster at runtime, with automatic rebalancing of data across the new topology.

### Implementation
- **File**: `internal/scaling.go`
- **Join Protocol**: New nodes contact seed nodes and broadcast membership
- **Rebalancing**: Keys are redistributed based on new hash ring

### Why It's Used

#### 1. **Horizontal Scalability**
```go
newNode := Node{ID: 4, Port: 5004}
cluster.addNode(newNode)
rebalanceRing() // Redistribute some keys to new node
```

#### 2. **Load Distribution**
New nodes automatically get their fair share of the load.

#### 3. **Zero-Downtime Scaling**
The system continues operating during membership changes.

### Example Scaling
```
Adding Node4 to 3-node cluster:
1. Node4 contacts Node1 (seed)
2. Node1 accepts join and broadcasts to all nodes
3. All nodes recompute hash ring with 4 nodes
4. ~25% of keys move to Node4
5. Node4 runs recovery to pull its assigned keys
```

---

## Algorithm Interactions

### How Algorithms Work Together

1. **Consistent Hashing** determines which nodes store each charging slot
2. **Quorum Replication** ensures data is stored on multiple nodes
3. **Primary Serialization** prevents double booking through ordered writes
4. **Heartbeat Detection** identifies when nodes fail
5. **Bully Election** selects new leaders when needed
6. **Crash Recovery** brings failed nodes back in sync
7. **Dynamic Membership** allows the cluster to scale

### Failure Scenario Walkthrough

```
1. Node3 crashes (detected by Heartbeat)
2. Bully Election makes Node2 the new leader
3. Quorum Replication continues with 2 nodes (W=2, R=2 still satisfied)
4. Client requests continue through remaining nodes
5. Node3 restarts and runs Crash Recovery
6. Node3 syncs with current cluster state
7. Node3 rejoins via Dynamic Membership protocol
8. Hash ring rebalances across all 3 nodes
```

---

## Design Trade-offs

### Why These Algorithms Were Chosen

| Algorithm | Advantage | Trade-off |
|-----------|-----------|-----------|
| Consistent Hashing | Minimal key movement on scaling | Slightly more complex than modulo hashing |
| Quorum Replication (N=3,W=2,R=2) | Tolerates 1 node failure, strong consistency | Higher latency than single-node writes |
| Primary Serialization | Simple conflict prevention | Primary can become bottleneck |
| Bully Election | Fast and simple | Requires unique IDs, higher ID wins |
| Heartbeat Detection | Quick failure detection | Network partitions can cause false positives |
| Crash Recovery | Automatic state sync | Recovery time depends on data size |
| Dynamic Membership | Zero-downtime scaling | Temporary inconsistencies during rebalancing |

### Alternative Algorithms Considered

- **Raft/Paxos**: More complex consensus, overkill for this use case
- **Gossip-based dissemination**: Slower convergence than heartbeat
- **Distributed locking**: More complex than primary serialization
- **Perfect hashing**: Not suitable for dynamic membership

---

## Conclusion

This distributed EV charging system demonstrates how multiple algorithms work together to create a fault-tolerant, scalable system. Each algorithm addresses specific distributed systems challenges:

- **Data Distribution**: Consistent hashing
- **Data Consistency**: Quorum replication + primary serialization
- **Fault Detection**: Heartbeat monitoring
- **Leadership**: Bully election
- **Recovery**: State transfer and anti-entropy
- **Scalability**: Dynamic membership and rebalancing

The combination provides a robust foundation for critical infrastructure like EV charging networks, where availability, consistency, and scalability are essential requirements.
