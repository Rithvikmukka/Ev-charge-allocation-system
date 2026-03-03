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

### Multi-device configuration

For multiple devices, you must configure:

- `--bind`: what interface the HTTP server listens on (use `0.0.0.0` to accept connections from other devices)
- `--advertise`: the IP/hostname that other devices should use to reach this node
- `--peers`: comma-separated list of *reachable* `host:port` peers (use LAN IPs, not `localhost`)

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
- A new node calls a seed using `POST /join?seed=<seedHost:port>`.
- The seed accepts the join (`POST /internal/join`) and **broadcasts** a membership update to all nodes.
- All nodes recompute the ring.
- The joining node runs a **recovery/rebalance pass** (re-uses Phase 8) to pull keys it is now responsible for.

### Demo: add Node4 to a 3-node cluster

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

Terminal 4 (start Node4 with a seed peer in `--peers`):

```powershell
go run ./cmd --id=4 --port=5004 --peers=1@localhost:5001
```

Now tell Node4 to join using Node1 as seed:

```powershell
curl.exe -X POST "http://localhost:5004/join?seed=localhost:5001"
```

Observe:

- Node1 logs `accepted join` and broadcasts membership.
- Nodes log `applied membership update`.
- Node4 logs it joined and runs recovery/rebalance.

You can query membership from any node:

```powershell
curl.exe "http://localhost:5001/membership"
```

## Next phases

- Phase 2: consistent hashing and replica selection
- Phase 3-4: replication + quorum reads/writes
- Phase 6-7: heartbeat failure detection + bully leader election
- Phase 8-9: crash recovery + horizontal scaling/rebalancing
