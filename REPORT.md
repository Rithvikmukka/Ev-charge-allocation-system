# Project Report: Distributed EV Charging Slot Allocation System

## Repository

https://github.com/Rithvikmukka/Ev-charge-allocation-system

## 1) Overview

This project is a terminal-based distributed systems simulation written in Go. Each running process represents a **node** in a cluster. Together, the nodes manage EV charging slot reservations across multiple stations and time slots.

The system focuses on:

- Replicating slot state across nodes
- Preventing double-booking under concurrency
- Handling node failures (crash detection, leader changes)
- Allowing nodes to rejoin and scale out dynamically

## 2) Problem Statement

In a real EV charging network, many users may try to reserve the same charging slot at the same time. A robust backend must:

- Avoid allocating the same slot to multiple vehicles
- Keep data available when nodes fail
- Recover automatically when nodes restart
- Scale horizontally as demand grows

## 3) What the System Provides

### Slot model

Slots are represented as keys like:

- `StationA-Slot1`
- `StationB-Slot2`
- `StationC-Slot3`

Each slot stores state:

- `Status` (e.g., `FREE`, `BOOKED`)
- `VehicleID`
- `Version` (monotonic version used for reads/read-repair)

### API surface (high-level)

- `POST /reserve` (reserve a slot)
- `POST /release` (release a slot)
- `GET /slot?slot=<SlotKey>` (read slot)
- `GET /membership` (view cluster membership)

Internally, nodes also call endpoints like `/internal/*` for replication, primary forwarding, membership updates, etc.

## 4) Distributed Architecture (Conceptual)

- Multiple identical nodes (same binary)
- Nodes communicate over HTTP
- Keys (slot IDs) are mapped to nodes using consistent hashing
- Each key is replicated to `N` nodes (replication factor)

A node receiving a client request will:

- Determine the replica set for the key
- Forward to the **primary** node for that key (for writes)
- Perform quorum reads/writes for consistency

## 5) Algorithms Used (and why)

A detailed algorithm explanation is available in `ALGORITHMS.md`. Summary:

### 5.1 Consistent hashing (`internal/hashing.go`)

- Used to map each slot key to a **primary node** and a replica list.
- Helps keep rebalancing small when nodes join/leave.

### 5.2 Replication + quorum reads/writes (`internal/replication.go`, `internal/quorum.go`)

Configured as:

- `N = 3` replicas
- `W = 2` write quorum
- `R = 2` read quorum

Why:

- With `W + R > N`, reads overlap with writes, so reads are much less likely to return stale values.
- The system can tolerate one replica being down and still succeed for reads/writes (depending on which replicas are reachable).

### 5.3 Primary serialization (strict booking)

- Prevents double booking by routing writes through a single primary for a key.
- Avoids needing a full consensus protocol for every booking operation.

### 5.4 Heartbeat failure detection (`internal/heartbeat.go`)

- Nodes monitor each other and detect failures.
- Failure events trigger membership changes and recovery behavior.

### 5.5 Bully leader election (`internal/election.go`)

- When a leader fails, nodes elect a new leader based on highest ID.
- Keeps coordination logic moving forward during failures.

### 5.6 Crash recovery/state transfer (`internal/recovery.go`)

- When a node restarts, it pulls/repairs state so it does not serve stale slot data.

### 5.7 Dynamic membership + rebalancing (`internal/scaling.go`)

- New nodes can join and the cluster rebalances responsibility.
- Supports a “zero-config” style join using a seed peer list (`--auto-join`).

## 6) How to Run

### Prerequisites

- Go `1.22+` (see `go.mod`)
- Windows PowerShell is supported (examples below)

### 6.1 Sanity checks

```powershell
go test ./...
go test -race ./...
go vet ./...
```

### 6.2 Run a 3-node cluster (single machine)

Open **three terminals**.

Terminal 1:

```powershell
go run ./cmd --id=1 --port=5001 --peers=2@localhost:5002,3@localhost:5003
```

Terminal 2:

```powershell
go run ./cmd --id=2 --port=5002 --peers=1@localhost:5001,3@localhost:5003
```

Terminal 3:

```powershell
go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002
```

### 6.3 Test a booking

Reserve:

```powershell
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body
```

Read:

```powershell
curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"
```

Release:

```powershell
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5002/release" -Method Post -ContentType "application/json" -Body $body
```

### 6.4 Multi-device / “zero-config” mode

Seed device:

```bash
go run ./cmd --peers= --auto-join
```

Joiner devices:

```bash
go run ./cmd --peers=<SEED_IP> --auto-join
```

Notes:

- For multi-device runs, you may need `--bind=0.0.0.0` and `--advertise=<LAN-IP>` (documented in `DEMO.md`).

## 7) Demo Scenarios Covered

`DEMO.md` is a step-by-step runbook that demonstrates:

- Normal booking and replication
- Concurrent booking (no double-booking)
- Node crash detection (heartbeats)
- Leader crash and election (bully)
- Node restart and recovery
- Adding a new node and observing rebalancing

## 7.1) Diagrams

![alt text](Architecture_diagram.png)

![alt text](Sequence_diagram.png)

## 8) Notes on Run Instructions (README vs DEMO)

### Is clear information present on how to run the system?

Yes.

- `README.md` includes multiple run modes (single-machine + multi-device + testing via curl/PowerShell).
- `DEMO.md` is clearer as a *linear script* for a live demonstration (start nodes → do booking → simulate failures → recover → scale).

### Should `README.md` and `DEMO.md` be merged?

I would **not fully merge** them. They serve different purposes:

- **`README.md`** should stay short and “entry-level”:
  - What the project is
  - What algorithms/features exist
  - The minimal commands to start a cluster and try one reserve/read/release
  - Links to deeper docs (`DEMO.md`, `ALGORITHMS.md`)

- **`DEMO.md`** should remain a dedicated runbook:
  - Step-by-step scenario instructions
  - Expected outputs
  - Failure simulation steps

A good compromise (optional) is:

- Keep both files
- Add a small section in `README.md` like “Quick Start” + “Full demo script: see `DEMO.md`” (this is already present)

## 9) Limitations / Assumptions

- This is a simulation intended for learning and demonstration.
- It uses a primary-per-key approach with quorums; it is not a full consensus-based datastore.
- Network partitions can cause quorum failures (by design) when `R` or `W` cannot be satisfied.

## 10) Future Improvements

- Add metrics (latency, quorum success rate, failure counts)
- Add a lightweight UI client (optional)
- Add configurable quorum parameters via flags
- Add more detailed observability/log correlation between nodes

---

## Appendix: Where key distribution is implemented

Key-to-node mapping is done in:

- `internal/hashing.go`

Main functions:

- `NewHashRing()`
- `PrimaryNode(key)`
- `ReplicaNodes(key, replicationFactor)`
