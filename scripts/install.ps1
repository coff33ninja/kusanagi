param(
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUTF8 = "1"

Write-Host "=== Kusanagi Install ===" -ForegroundColor Cyan

# 1. Check/install uv
$uvPath = Get-Command "uv" -ErrorAction SilentlyContinue
if (-not $uvPath) {
    Write-Host "Installing uv..." -ForegroundColor Yellow
    if (-not $DryRun) {
        & "$ProjectRoot\.ai_scripts\ensure-uv.ps1" -InstallMissing
    }
} else {
    Write-Host "uv found: $($uvPath.Source)" -ForegroundColor Green
}

# 2. Install espeak-ng (required by TTS phonemizer)
$espeakDll = "C:\Program Files\eSpeak NG\libespeak-ng.dll"
$espeakPath = Get-Command "espeak-ng" -ErrorAction SilentlyContinue
if (-not $espeakPath -and -not (Test-Path $espeakDll)) {
    Write-Host "Installing eSpeak-NG via winget..." -ForegroundColor Yellow
    if (-not $DryRun) {
        winget install "eSpeak NG" --accept-source-agreements --accept-package-agreements
        if ($LASTEXITCODE -ne 0) {
            Write-Host "winget install failed. Try manual install from:" -ForegroundColor Red
            Write-Host "  https://github.com/espeak-ng/espeak-ng/releases" -ForegroundColor Yellow
            exit 1
        }
    }
} else {
    Write-Host "eSpeak-NG found: $($espeakPath.Source)" -ForegroundColor Green
}

# 3. Ensure PHONEMIZER_ESPEAK_LIBRARY env var is set
$espeakDll = "C:\Program Files\eSpeak NG\libespeak-ng.dll"
if (Test-Path $espeakDll) {
    [Environment]::SetEnvironmentVariable("PHONEMIZER_ESPEAK_LIBRARY", $espeakDll, "User")
    Write-Host "PHONEMIZER_ESPEAK_LIBRARY set in user environment." -ForegroundColor Green
} else {
    Write-Host "Warning: libespeak-ng.dll not found at $espeakDll" -ForegroundColor Yellow
}

# 4. Sync dependencies
Write-Host "Syncing Python dependencies..." -ForegroundColor Yellow
if (-not $DryRun) {
    $env:UV_CACHE_DIR = Join-Path $ProjectRoot ".uv\cache"
    $env:UV_LINK_MODE = "copy"
    uv sync --project $ProjectRoot
    if ($LASTEXITCODE -ne 0) {
        Write-Host "uv sync failed." -ForegroundColor Red
        exit 1
    }
    Write-Host "Python dependencies installed." -ForegroundColor Green
}

# 5. Check MCP servers
Write-Host "Checking MCP servers..." -ForegroundColor Yellow
$serverPath = Join-Path (Join-Path $ProjectRoot "servers") "mcp-server.exe"
if (-not (Test-Path $serverPath)) {
    Write-Host "mcp-server.exe not found. Run scripts\download-servers.ps1 to download it." -ForegroundColor Yellow
} else {
    $size = (Get-Item $serverPath).Length
    Write-Host "mcp-server.exe found ($([math]::Round($size / 1MB, 1)) MB)" -ForegroundColor Green
}

Write-Host "=== Install complete ===" -ForegroundColor Cyan
Write-Host "Run '.\scripts\run.ps1' for text mode or '.\scripts\run.ps1 voice' for voice mode." -ForegroundColor Cyan
