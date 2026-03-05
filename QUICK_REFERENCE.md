# Quick Reference Commands

## Cluster Operations

### Start Cluster (Zero-Config)
```bash
# Seed node
go run ./cmd --peers= --auto-join

# Joining nodes
go run ./cmd --peers=<SEED_IP> --auto-join
```

### Start Cluster (Manual)
```bash
# Node 1
go run ./cmd --id=1 --port=5001 --peers=2@localhost:5002,3@localhost:5003

# Node 2
go run ./cmd --id=2 --port=5002 --peers=1@localhost:5001,3@localhost:5003

# Node 3
go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002
```

### Slot Operations
```powershell
# Check slot status
curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"

# Book a slot
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body

# Release a slot
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/release" -Method Post -ContentType "application/json" -Body $body

# Check all slots
"StationA-Slot1","StationA-Slot2","StationA-Slot3","StationB-Slot1","StationB-Slot2","StationB-Slot3","StationC-Slot1","StationC-Slot2","StationC-Slot3" | ForEach-Object { curl.exe "http://localhost:5001/slot?slot=$_" }

# Release all slots
"StationA-Slot1","StationA-Slot2","StationA-Slot3","StationB-Slot1","StationB-Slot2","StationB-Slot3","StationC-Slot1","StationC-Slot2","StationC-Slot3" | ForEach-Object { 
    $body = @{ slot = $_; vehicle = "EV-RELEASE" } | ConvertTo-Json
    Invoke-RestMethod "http://localhost:5001/release" -Method Post -ContentType "application/json" -Body $body 
}
```

### Cluster Management
```powershell
# Check cluster membership
curl.exe "http://localhost:5001/membership"

# Check cluster health
curl.exe "http://localhost:5001/healthz"

# Trigger election
curl.exe -X POST "http://localhost:5001/election/start"

# Join cluster (runtime)
curl.exe -X POST "http://localhost:5004/join?seed=localhost:5001"
```

### Demo Scripts
```powershell
# Concurrent booking demo
Start-Job -ScriptBlock {
    param($Slot)
    $body1 = @{ slot = $Slot; vehicle = "EV-DEMO-1" } | ConvertTo-Json
    $body2 = @{ slot = $Slot; vehicle = "EV-DEMO-2" } | ConvertTo-Json
    
    $job1 = Start-Job -ScriptBlock { ScriptBlock = { Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body1 } }
    $job2 = Start-Job -ScriptBlock { ScriptBlock = { Invoke-RestMethod "http://localhost:5002/reserve" -Method Post -ContentType "application/json" -Body $body2 } }
    
    $job1, $job2 | Wait-Job
}

# Usage
Start-Job -ScriptBlock -Slot "StationB-Slot1"
```

### Multi-Device Configuration
```bash
# Find your IP
ipconfig

# Start with auto-detection
go run ./cmd --peers= --auto-join

# Manual IP specification
go run ./cmd --advertise=10.12.234.60 --port=5001 --peers=10.12.233.137:5002,10.12.226.62:5003 --auto-join
```

### Testing
```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Vet code
go vet ./...

# Build
go build -o ev-charging ./cmd
```

### Web Dashboard
```bash
# Open dashboard
# Open browser to: http://localhost:5001/web/index.html

# Or serve static files
go run ./cmd --peers= --auto-join &
python -m http.server 8000 --directory web/
```

## Troubleshooting

### Port Already in Use
```bash
# Windows
netstat -ano | findstr :5001

# Linux/macOS
lsof -i :5001
```

### Connection Refused
```bash
# Check if node is running
curl.exe "http://localhost:5001/healthz"

# Check firewall
netsh advfirewall show allprofiles
netsh advfirewall show allprofiles state
```

### Common Errors
- `bind: address already in use` → Change port
- `connection refused` → Node not running or wrong port
- `timeout` → Network issues or node down
- `409 conflict` → Expected for concurrent booking

## File Locations
- Main code: `cmd/main.go`
- Core logic: `internal/`
- Tests: `internal/*_test.go`
- Dashboard: `web/`
- Documentation: `README.md`, `DEMO.md`, `QUICK_REFERENCE.md`

### 4-System Demo Instructions

#### Setup (3 Systems)
```bash
# System 1 (Seed)
go run ./cmd --peers= --auto-join

# System 2
go run ./cmd --peers=<SYSTEM1_IP> --auto-join

# System 3
go run ./cmd --peers=<SYSTEM1_IP> --auto-join
```

#### Add 4th System
```bash
# System 4 (joins existing cluster)
go run ./cmd --peers=<SYSTEM1_IP> --auto-join
```

#### Verify 4-System Cluster
```powershell
curl.exe "http://<SYSTEM1_IP>/membership"
```

#### Demo Sequence
1. **Normal booking** (book 2-3 slots across systems)
2. **Concurrent booking** (same slot on 2 systems simultaneously)
3. **Node crash** (stop System 3, observe failure detection)
4. **Leader election** (stop System 1, watch System 2 become leader)
5. **Node restart** (restart System 3, observe recovery)
6. **Scaling** (add System 4, observe rebalancing)

#### Expected Behaviors
- **Auto-join**: System 4 automatically joins via System 1
- **Rebalancing**: Keys redistribute when System 4 joins
- **Failure detection**: Systems 1-2 mark System 3 as failed
- **Election**: System 2 wins election (highest ID among 1,2)
- **Recovery**: System 3 syncs latest state on restart

## Git Commands
```bash
# Clone repository
git clone https://github.com/Rithvikmukka/Ev-charge-allocation-system.git

# Create feature branch
git checkout -b feature-name

# Add changes
git add .
git commit -m "Description"

# Push to origin
git push origin feature-name

# Merge to main
git checkout main
git merge feature-name
git push origin main
```
