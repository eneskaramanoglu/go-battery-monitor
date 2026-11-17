# Builds bin\batterymon.exe: a single static GUI exe (no console window, no cgo, no DLLs).
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$env:CGO_ENABLED = '0'
New-Item -ItemType Directory -Force bin | Out-Null
go vet ./...
go build -trimpath -ldflags "-s -w -H windowsgui" -o bin\batterymon.exe .\cmd\batterymon
$kb = [math]::Round((Get-Item bin\batterymon.exe).Length / 1KB)
Write-Host "Built bin\batterymon.exe ($kb KB)"
