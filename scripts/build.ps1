param(
    [switch]$Release,
    [string]$Output = "kusanagi.exe"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$ver = (Get-Content (Join-Path $ProjectRoot "VERSION") -Raw).Trim()

Write-Host "=== Kusanagi build ===" -ForegroundColor Cyan
Write-Host "Version: $ver" -ForegroundColor Gray

$go = Get-Command "go" -ErrorAction SilentlyContinue
if (-not $go) { Write-Host "Go not found." -ForegroundColor Red; exit 1 }

& (Join-Path $PSScriptRoot "gen-icons.ps1")
if (-not $?) { exit 1 }

$ldflags = "-s -w -X main.Version=$ver"
if (-not $Release) {
    $ldflags = "-X main.Version=$ver"
}

Write-Host "Building..." -ForegroundColor Gray
$env:CC = "zig cc"
$env:CGO_ENABLED = "1"
go build -ldflags="$ldflags" -o $Output .\cmd\kusanagi\
if (-not $?) { exit 1 }

$sizeBytes = (Get-Item $Output).Length
$mib = [math]::Round($sizeBytes / 1048576, 1)
Write-Host "OK: $Output ($mib MB)" -ForegroundColor Green
