# Distributed EV Charging Slot Allocation System

Terminal-based distributed systems simulation in Go.

## Demo runbook

See `DEMO.md` for a step-by-step, terminal-based demo script covering:

- Normal booking
- Concurrent booking (no double booking)
- Node crash + heartbeat failure detection
- Leader crash + bully election
- Node restart + crash recovery
- New node join + rebalancing

## Single system vs multiple systems

This codebase is **one node program**. You run the same program on:

- multiple terminals on one machine (simulating multiple laptops), or
- multiple physical devices on the same network.

Each running process is a distinct **node** in the cluster.

### Zero-config startup (recommended)

For multi-device deployment, the system now supports **zero-config startup**:

- **Auto IP detection**: Automatically detects the first non-loopback IPv4 address
- **Auto port assignment**: Uses `5000 + ID` (e.g., ID 60 → port 5060)
- **Auto ID assignment**: Uses last octet of IP as node ID (e.g., `10.12.234.60` → ID 60)
- **Auto-join**: New nodes automatically join via the first reachable peer

#### Quick start (3 laptops)

```bash
# Laptop A (seed)
go run ./cmd --peers= --auto-join

# Laptop B and C (joiners)
go run ./cmd --peers=10.12.234.60 --auto-join
go run ./cmd --peers=10.12.234.60 --auto-join
```

Replace `10.12.234.60` with the seed laptop’s actual IP.

#### Manual configuration (if needed)

For multiple devices, you can still configure manually:

- `--bind`: what interface the HTTP server listens on (use `0.0.0.0` to accept connections from other devices)
- `--advertise`: the IP/hostname that other devices should use to reach this node
- `--peers`: comma-separated list of *reachable* `host:port` peers (use LAN IPs, not `localhost`)

## Algorithms used

| Algorithm | Purpose | Implementation |
|-----------|---------|----------------|
| **Consistent Hashing** | Distribute slots across nodes | `internal/hashing.go` |
| **Quorum Replication** | Ensure data consistency despite failures | `internal/replication.go` (N=3, W=2, R=2) |
| **Primary Serialization** | Prevent double booking | Primary node handles all writes for a slot |
| **Heartbeat Failure Detection** | Detect node crashes | `internal/heartbeat.go` |
| **Bully Leader Election** | Elect new leader when current fails | `internal/election.go` |
| **Crash Recovery/State Transfer** | Sync restarted node with latest data | `internal/recovery.go` |
| **Dynamic Membership/Rebalancing** | Add/remove nodes and redistribute data | `internal/scaling.go` |

## Phase 1 (implemented): Node Setup

**Distributed concept:** decentralized, identical nodes (no special “master” at boot).

### Run (multiple terminals)

Terminal 1:

```bash
go run ./cmd --id=1 --port=5001
```

Terminal 2:

```bash
go run ./cmd --id=2 --port=5002 --peers=localhost:5001
```

Terminal 3:

```bash
go run ./cmd --id=3 --port=5003 --peers=localhost:5001,localhost:5002
```

### What you should see

Each node prints:

- Node ID / Port
- Hash token (placeholder for consistent hashing in Phase 2)
- Membership list (static for now)
- Local storage initialized with 9 slot keys (3 stations x 3 slots)

## Phase 3 (implemented): Replication (HTTP)

**Distributed concept:** replication and fault tolerance (writes are fanned out to the `N` replica nodes selected by consistent hashing).

### Run (3 nodes)

Terminal 1:

```bash
go run ./cmd --id=1 --port=5001 --peers=localhost:5002,localhost:5003
```

Terminal 2:

```bash
go run ./cmd --id=2 --port=5002 --peers=localhost:5001,localhost:5003
```

Terminal 3:

```bash
go run ./cmd --id=3 --port=5003 --peers=localhost:5001,localhost:5002
```

### Test booking

Read slot state:

```bash
curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"
```

Reserve:

```bash
curl.exe -X POST http://localhost:5001/reserve -H "Content-Type: application/json" -d "{\"slot\":\"StationA-Slot1\",\"vehicle\":\"EV101\"}"
```

Release:

```bash
curl.exe -X POST http://localhost:5002/release -H "Content-Type: application/json" -d "{\"slot\":\"StationA-Slot1\",\"vehicle\":\"EV101\"}"
```

Notes for Windows/PowerShell:

- Keep each `go run ...` node running in its own terminal. If you see exit code `0xc000013a`, the process was interrupted (Ctrl+C / terminal stop), so `localhost:5001` will refuse connections.
- In PowerShell, `curl` is often an alias for `Invoke-WebRequest`, which does not accept `-H` and `-d` like real curl. Use `curl.exe` explicitly (as shown above), or use `Invoke-RestMethod`.

PowerShell alternative (Invoke-RestMethod):

```powershell
Invoke-RestMethod "http://localhost:5001/slot?slot=StationA-Slot1" -Method Get

$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body

$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5002/release" -Method Post -ContentType "application/json" -Body $body
```

Observe logs in all terminals:

- The coordinator prints the replica set it is writing to.
- Replicas accept the internal write on `POST /internal/apply`.
- Phase 4 adds quorum (`W`/`R`) and read-repair.

## Phase 4 (implemented): Quorum Reads/Writes (N=3, W=2, R=2)

**Distributed concept:** quorum-based consistency.

### What changed

- `POST /reserve` and `POST /release` now require **write quorum**: at least `W=2` replica acknowledgments.
- `GET /slot` now requires **read quorum**: at least `R=2` replica responses. It returns the **latest Version** among the responses and performs **read-repair** on stale responders.

### Demo 1: Write quorum survives one node down

1) Start Node1 + Node2, but do NOT start Node3.

2) Reserve (should still succeed because `W=2` can be satisfied by Node1+Node2):

```powershell
$body = @{ slot = "StationB-Slot1"; vehicle = "EV201" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body
```

3) If only one node is up, the same request should fail with `write quorum not satisfied`.

### Demo 2: Read quorum behavior

- With only one node up, `GET /slot` should fail with `read quorum not satisfied (R=2 ...)`.
- With two nodes up, `GET /slot` should succeed.

### Demo 3: Trigger read-repair (educational)

1) Start all 3 nodes.

2) Kill Node3.

3) Reserve a slot (this will update Node1+Node2, Node3 misses it).

4) Restart Node3 (it is now stale).

5) Read the slot using `GET /slot` from any node. You should see a log like `read-repair ... pushing version=...` and Node3 will be updated.

## Phase 9 (implemented): Horizontal Scaling (Dynamic Join + Rebalancing)

**Distributed concept:** horizontal scaling + dynamic membership + load redistribution.

### How it works

- `--peers` is a **seed list**.
- With `--auto-join`, new nodes automatically contact the first seed and join.
- The seed accepts the join (`POST /internal/join`) and **broadcasts** a membership update to all nodes.
- All nodes recompute the ring.
- The joining node runs a **recovery/rebalance pass** (re-uses Phase 8) to pull keys it is now responsible for.

### Demo: add Node4 to a 3-node cluster (zero-config)

Terminal 1 (seed):

```bash
go run ./cmd --peers= --auto-join
```

Terminal 2:

```bash
go run ./cmd --peers=10.12.234.60 --auto-join
```

Terminal 3:

```bash
go run ./cmd --peers=10.12.234.60 --auto-join
```

Terminal 4 (new node):

```bash
go run ./cmd --peers=10.12.234.60 --auto-join
```

Observe:
- Node1 logs `accepted join` and broadcasts membership.
- Nodes log `applied membership update`.
- Node4 logs it joined and runs recovery/rebalance.

You can query membership from any node:

```bash
curl.exe "http://10.12.234.60:5001/membership"
```

### Manual join (if needed)

If auto-join is disabled, manually trigger a join:

```bash
curl.exe -X POST "http://localhost:5004/join?seed=localhost:5001"
```

## Next phases

- Phase 2: consistent hashing and replica selection
- Phase 3-4: replication + quorum reads/writes
- Phase 6-7: heartbeat failure detection + bully leader election
- Phase 8-9: crash recovery + horizontal scaling/rebalancing

## Architecture Diagram

![alt text](<Architecture_diagram.png>)

## Sequence Diagram

![alt text](<Sequence_diagram.png>)