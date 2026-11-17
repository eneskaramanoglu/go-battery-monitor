# Builds a single static GUI exe (no console window, no cgo, no DLLs).
#   .\build.ps1                                local build: bin\batterymon.exe, version "dev"
#   .\build.ps1 -Version v0.1.0                release build (also stamps the exe resources)
#   .\build.ps1 -Version v0.1.0 -Arch arm64    cross-compile for Windows on ARM
param(
    [string]$Version = 'dev',
    [ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64',
    [string]$Out = ''
)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = $Arch
if (-not $Out) { $Out = if ($Arch -eq 'amd64') { 'bin\batterymon.exe' } else { "bin\batterymon-$Arch.exe" } }
$commit = (git rev-parse --short HEAD 2>$null)
New-Item -ItemType Directory -Force (Split-Path $Out) | Out-Null

if ($Version -match '^v?(\d+)\.(\d+)\.(\d+)$') {
    $num = "$($Matches[1]).$($Matches[2]).$($Matches[3]).0"
    $env:GOARCH = ''   # go-winres itself runs on the host
    go tool go-winres make --in winres/winres.json --out cmd/batterymon/rsrc --arch amd64,arm64 `
        --file-version $num --product-version $Version
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    $env:GOARCH = $Arch
}

go vet ./...
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$pkg = 'github.com/eneskaramanoglu/go-battery-monitor/internal/version'
go build -trimpath -ldflags "-s -w -H windowsgui -X $pkg.Version=$Version -X $pkg.Commit=$commit" -o $Out .\cmd\batterymon
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$kb = [math]::Round((Get-Item $Out).Length / 1KB)
Write-Host "Built $Out $Version $Arch ($kb KB)"
