# Builds bin\batterymon.exe: a single static GUI exe (no console window, no cgo, no DLLs).
#   .\build.ps1                    local build, version "dev"
#   .\build.ps1 -Version v0.1.0    release build
param([string]$Version = 'dev')
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$env:CGO_ENABLED = '0'
$commit = (git rev-parse --short HEAD 2>$null)
New-Item -ItemType Directory -Force bin | Out-Null
go vet ./...
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$pkg = 'github.com/eneskaramanoglu/go-battery-monitor/internal/version'
go build -trimpath -ldflags "-s -w -H windowsgui -X $pkg.Version=$Version -X $pkg.Commit=$commit" -o bin\batterymon.exe .\cmd\batterymon
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$kb = [math]::Round((Get-Item bin\batterymon.exe).Length / 1KB)
Write-Host "Built bin\batterymon.exe $Version ($kb KB)"
