# demo-all-features.ps1
# Automated demo of all distributed features (Phases 1–9)

Write-Host "=== Starting 3-node cluster ==="
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=1 --port=5001 --peers=2@localhost:5002,3@localhost:5003"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=2 --port=5002 --peers=1@localhost:5001,3@localhost:5003"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002"
Start-Sleep -Seconds 6

Write-Host "`n=== 1) Normal booking (replication + quorum) ==="
$body = @{ slot = "StationA-Slot1"; vehicle = "EV101" } | ConvertTo-Json
Invoke-RestMethod "http://localhost:5001/reserve" -Method Post -ContentType "application/json" -Body $body
curl.exe "http://localhost:5001/slot?slot=StationA-Slot1"

Write-Host "`n=== 2) Concurrent booking (no double booking) ==="
$bodyA = @{ slot = "StationA-Slot2"; vehicle = "EV201" } | ConvertTo-Json
$bodyB = @{ slot = "StationA-Slot2"; vehicle = "EV202" } | ConvertTo-Json
$jobA = Start-Job -ScriptBlock { param($url,$b) try { Invoke-RestMethod $url -Method Post -ContentType "application/json" -Body $b } catch { $_ } } -ArgumentList "http://localhost:5002/reserve",$bodyA
$jobB = Start-Job -ScriptBlock { param($url,$b) try { Invoke-RestMethod $url -Method Post -ContentType "application/json" -Body $b } catch { $_ } } -ArgumentList "http://localhost:5003/reserve",$bodyB
Wait-Job -Job $jobA,$jobB -Timeout 5 | Out-Null
Receive-Job -Job $jobA; Receive-Job -Job $jobB
Remove-Job -Job $jobA,$jobB -Force

Write-Host "`n=== 3) Node crash (heartbeat failure detection) ==="
Write-Host "Kill Node3 window to see heartbeat timeout logs on Nodes 1 and 2."
Read-Host "Press Enter when Node3 is stopped..."

Write-Host "`n=== 4) Leader crash (bully election) ==="
Write-Host "Kill Node2 (current leader) to trigger election. Node1 should become leader."
Read-Host "Press Enter when Node2 is stopped..."

Write-Host "`n=== 5) Node restart (crash recovery) ==="
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002"
Start-Sleep -Seconds 3
Write-Host "Node3 restarted; check logs for Phase 8 recovery updates."

Write-Host "`n=== 6) New node join (scaling + rebalancing) ==="
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=4 --port=5004 --peers=1@localhost:5001"
Start-Sleep -Seconds 3
curl.exe -X POST "http://localhost:5004/join?seed=localhost:5001"
Start-Sleep -Seconds 2
curl.exe "http://localhost:5001/membership"

Write-Host "`n=== Demo complete ==="
Write-Host "Keep node windows open to continue experimenting."
