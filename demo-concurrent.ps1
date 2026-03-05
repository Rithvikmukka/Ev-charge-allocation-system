# demo-concurrent.ps1
# Starts 3 nodes in background windows and runs the concurrent booking demo

Write-Host "=== Starting 3-node cluster in background windows ==="

# Start Node 1
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=1 --port=5001 --peers=2@localhost:5002,3@localhost:5003"

# Start Node 2
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=2 --port=5002 --peers=1@localhost:5001,3@localhost:5003"

# Start Node 3
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; go run ./cmd --id=3 --port=5003 --peers=1@localhost:5001,2@localhost:5002"

Write-Host "Waiting 5 seconds for nodes to start..."
Start-Sleep -Seconds 5

Write-Host "=== Running concurrent booking demo ==="
Write-Host "Sending two reserve requests for the same slot (StationA-Slot2) simultaneously..."

# Prepare bodies
$bodyA = @{ slot = "StationA-Slot2"; vehicle = "EV201" } | ConvertTo-Json
$bodyB = @{ slot = "StationA-Slot2"; vehicle = "EV202" } | ConvertTo-Json

# Run both requests in parallel using jobs
$jobA = Start-Job -ScriptBlock {
    param($url, $body)
    try {
        $resp = Invoke-RestMethod $url -Method Post -ContentType "application/json" -Body $body
        Write-Output "Request A SUCCESS: $resp"
    } catch {
        Write-Output "Request A FAILED: $($_.Exception.Message)"
    }
} -ArgumentList "http://localhost:5002/reserve", $bodyA

$jobB = Start-Job -ScriptBlock {
    param($url, $body)
    try {
        $resp = Invoke-RestMethod $url -Method Post -ContentType "application/json" -Body $body
        Write-Output "Request B SUCCESS: $resp"
    } catch {
        Write-Output "Request B FAILED: $($_.Exception.Message)"
    }
} -ArgumentList "http://localhost:5003/reserve", $bodyB

# Wait for both jobs
$null = Wait-Job -Job $jobA, $jobB -Timeout 10

# Get results
Receive-Job -Job $jobA
Receive-Job -Job $jobB

# Cleanup jobs
Remove-Job -Job $jobA, $jobB -Force

Write-Host "=== Verifying state consistency across nodes ==="
curl.exe "http://localhost:5001/slot?slot=StationA-Slot2"
curl.exe "http://localhost:5002/slot?slot=StationA-Slot2"
curl.exe "http://localhost:5003/slot?slot=StationA-Slot2"

Write-Host "Demo complete. Keep the node windows open to continue experimenting."
