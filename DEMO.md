# Demo Script (Terminal-Based)

This script walks through the required demo scenarios. Run nodes in separate terminals.

## 0) Sanity checks

```powershell
go test ./...
go test -race ./...
go vet ./...
```

## 1) Start a 3-node cluster

### Option A: Single machine (localhost)

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

### Option B: Multiple laptops (zero-config)

Find each laptop’s Wi‑Fi IP (`ipconfig`), then:

**Laptop A (seed):**
```bash
go run ./cmd --peers= --auto-join
```

**Laptop B and C (joiners):**
```bash
go run ./cmd --peers=10.12.234.60 --auto-join
go run ./cmd --peers=10.12.234.60 --auto-join
```

Replace `10.12.234.60` with the seed laptop’s IP.

## 2) Normal booking (reserve + replicated reads)

```powershell
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body

curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"
curl.exe "http://localhost:5002/slot?slot=StationA-Slot1"
curl.exe "http://localhost:5003/slot?slot=StationA-Slot1"
```

For multi-device, replace `localhost:5001` with the seed’s IP:port.

Expected:
- All nodes show `BOOKED`, `EV101`, same `Version`.
- Logs show Phase 5 primary forwarding and quorum acks.

## 3) Concurrent booking (two terminals)

In Terminal A:

```powershell
$body = @{ slot = "StationA-Slot2"; vehicle = "EV201" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body
```

In Terminal B (run immediately after):

```powershell
$body = @{ slot = "StationA-Slot2"; vehicle = "EV202" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5002/reserve" -Method Post -ContentType "application/json" -Body $body
```

For multi-device, run each command on different laptops using their IP:port.

Expected:
- One request succeeds (200), one fails (409 slot already booked)
- No double booking: both nodes agree on final state

## 4) Node crash + heartbeat failure detection

1. Stop Node3 (Ctrl+C in Terminal 3)
2. Watch Terminals 1 and 2 for heartbeat logs
3. After ~5 seconds, you should see: `Heartbeat missed from ... -> MARK FAILED`

## 5) Leader crash + bully election

1. With Node3 down, Node2 is now leader (highest ID)
2. Stop Node2 (Ctrl+C in Terminal 2)
3. Watch Terminal 1 for election messages
4. Node1 should become leader and announce it

## 6) Node restart + crash recovery

1. Restart Node3:
```powershell
go run ./cmd --id=3 --port=5003 --peers=localhost:5001,localhost:5002
```

For multi-device:
```bash
go run ./cmd --peers=10.12.234.60 --auto-join
```

2. Watch logs for recovery messages like:
`Recovery: slot=... updated localVersion=... -> ...`

## 7) Horizontal scaling (add Node4)

Terminal 4:

```powershell
go run ./cmd --id=4 --port=5004 --peers=localhost:5001
```

For multi-device (zero-config):
```bash
go run ./cmd --peers=10.12.234.60 --auto-join
```

Verify membership:
```powershell
curl.exe "http://localhost:5001/membership"
```

You should see 4 nodes.

Expected:
- Seed logs `accepted join`.
- Other nodes log `applied membership update`.
- Node4 logs join and recovery/rebalance.

## Multi-device note

If running across laptops:
- use `--bind=0.0.0.0`
- use `--advertise=<LAN-IP>`
- use `--peers=<id@LAN-IP:port,...>`
