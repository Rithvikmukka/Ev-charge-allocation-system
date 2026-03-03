# Demo Script (Terminal-Based)

This script walks through the required demo scenarios. Run nodes in separate terminals.

## 0) Sanity checks

```powershell
go test ./...
go test -race ./...
go vet ./...
```

## 1) Start a 3-node cluster

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

## 2) Normal booking (reserve + replicated reads)

```powershell
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body

curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"
curl.exe "http://localhost:5002/slot?slot=StationA-Slot1"
curl.exe "http://localhost:5003/slot?slot=StationA-Slot1"
```

Expected:
- All nodes show `BOOKED`, `EV101`, same `Version`.
- Logs show Phase 5 primary forwarding and quorum acks.

## 3) Concurrent booking (two terminals)

In Terminal A:

```powershell
$body = @{ slot = "StationA-Slot2"; vehicle = "EV201" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5002/reserve" -Method Post -ContentType "application/json" -Body $body
```

In Terminal B (run quickly at the same time):

```powershell
$body = @{ slot = "StationA-Slot2"; vehicle = "EV202" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5003/reserve" -Method Post -ContentType "application/json" -Body $body
```

Expected:
- One request returns `200`.
- The other returns `409` (no double booking).

## 4) Node crash (failure detection)

- Stop Node3 (Ctrl+C in Terminal 3).

Expected within ~5 seconds:
- Nodes log Phase 6 heartbeat timeout and mark Node3 FAILED.

## 5) Leader crash (bully election)

- With Node3 down (highest ID), expect Node2 to become leader.

Optional manual trigger:

```powershell
curl.exe -X POST "http://localhost:5001/election/start"
```

Expected:
- Logs show election messages and leader announcement.

## 6) Node restart (crash recovery)

- Restart Node3:

```powershell
go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002
```

Expected:
- Phase 8 recovery logs show stale keys updated.

## 7) Horizontal scaling: join Node4 at runtime

Terminal 4:

```powershell
go run ./cmd --id=4 --port=5004 --peers=1@localhost:5001
```

Then:

```powershell
curl.exe -X POST "http://localhost:5004/join?seed=localhost:5001"
```

Verify membership:

```powershell
curl.exe "http://localhost:5001/membership"
```

Expected:
- Seed logs `accepted join`.
- Other nodes log `applied membership update`.
- Node4 logs join and recovery/rebalance.

## Multi-device note

If running across laptops:
- use `--bind=0.0.0.0`
- use `--advertise=<LAN-IP>`
- use `--peers=<id@LAN-IP:port,...>`
