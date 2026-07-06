param(
    [string]$Mode = "chat"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$GoBin = Join-Path $ProjectRoot "kusanagi.exe"
$ConfigPath = Join-Path $ProjectRoot "config.json"
$ServerPath = Join-Path $ProjectRoot "servers\mcp-server.exe"

Write-Host "=== Kusanagi (Go) ===" -ForegroundColor Cyan

# 1. Check config
if (-not (Test-Path $ConfigPath)) {
    Write-Host "config.json not found at $ConfigPath" -ForegroundColor Red
    Write-Host "Copy config.example.json to config.json and add your API keys." -ForegroundColor Yellow
    exit 1
}

# 2. Check MCP server binary
if (-not (Test-Path $ServerPath)) {
    Write-Host "mcp-server.exe not found at $ServerPath" -ForegroundColor Red
    Write-Host "Run scripts\download-servers.ps1 to download it." -ForegroundColor Yellow
    exit 1
}

# 3. Check Go binary
if (-not (Test-Path $GoBin)) {
    Write-Host "kusanagi.exe not found at $GoBin" -ForegroundColor Yellow
    Write-Host "Building Go binary..." -ForegroundColor Cyan
    $ver = (Get-Content (Join-Path $ProjectRoot "VERSION") -Raw).Trim()
    Push-Location $ProjectRoot
    try {
        $env:CC = "zig cc"
        $env:CGO_ENABLED = "1"
        go build -ldflags="-X main.Version=$ver" -o kusanagi.exe .\cmd\kusanagi\
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Go build failed." -ForegroundColor Red
            exit 1
        }
        Write-Host "Build complete." -ForegroundColor Green
    } finally {
        Pop-Location
    }
}

# 4. Validate binary
$size = (Get-Item $GoBin).Length
$version = & $GoBin --version 2>$null
if (-not $?) {
    $version = (Get-Item $GoBin).CreationTime.ToString("yyyy-MM-dd")
}
Write-Host "Binary: $([math]::Round($size / 1MB, 1)) MB (built $version)" -ForegroundColor Green

switch ($Mode) {
    "chat" {
        Write-Host "Starting Kusanagi (Go)..." -ForegroundColor Cyan
        & $GoBin -config $ConfigPath
    }
    "voice" {
        Write-Host "Starting Kusanagi (Go) with voice I/O..." -ForegroundColor Cyan
        & $GoBin -config $ConfigPath
    }
    default {
        Write-Host "Unknown mode: $Mode. Use 'chat' or 'voice'." -ForegroundColor Red
        exit 1
    }
}
