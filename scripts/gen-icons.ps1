param(
    [switch]$Force
)

<#
.SYNOPSIS
    Generate app.ico from app.svg, then compile into a Windows icon resource (.syso)
    for embedding into kusanagi.exe.
.DESCRIPTION
    Run before build. Uses a Go SVG rasterizer to produce PNGs + ICO, then rsrc
    to compile app.ico into a COFF object that the Go linker embeds automatically.
#>

$ErrorActionPreference = "Stop"
$repoRoot    = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$svgPath     = Join-Path (Join-Path $repoRoot "icons") "app.svg"
$icoPath     = Join-Path (Join-Path $repoRoot "icons") "app.ico"
$toolDir     = Join-Path $PSScriptRoot "gen-iconstool"
$sysoOut     = Join-Path (Join-Path (Join-Path $repoRoot "cmd") "kusanagi") "rsrc_windows.syso"

if (-not (Test-Path $svgPath)) {
    Write-Error "SVG not found at $svgPath"
    exit 1
}

# Step 1: Generate ICO from SVG
Push-Location $toolDir
try {
    Write-Host "=== Generating ICO from SVG ===" -ForegroundColor Cyan
    go run . -svg="$svgPath" -ico="$icoPath"
    if (-not $?) { exit 1 }
} finally {
    Pop-Location
}

# Step 2: Install rsrc if not present
$rsrc = Get-Command "rsrc" -ErrorAction SilentlyContinue
if (-not $rsrc) {
    Write-Host "Installing rsrc (github.com/akavel/rsrc)..." -ForegroundColor Gray
    go install github.com/akavel/rsrc@latest
    $env:PATH = "$env:USERPROFILE\go\bin;$env:PATH"
    $rsrc = Get-Command "rsrc" -ErrorAction SilentlyContinue
    if (-not $rsrc) {
        Write-Error "rsrc not found after install"
        exit 1
    }
}

# Step 3: Compile ICO to .syso
$sysoDir = Split-Path $sysoOut -Parent
if (-not (Test-Path $sysoDir)) {
    New-Item -ItemType Directory -Path $sysoDir -Force | Out-Null
}

Write-Host "=== Generating .syso from ICO ===" -ForegroundColor Cyan
& rsrc -ico $icoPath -o $sysoOut
if (-not $?) { exit 1 }

Write-Host "OK: $sysoOut" -ForegroundColor Green
