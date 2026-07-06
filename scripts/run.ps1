param(
    [string]$Mode = "chat"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUTF8 = "1"

$env:UV_CACHE_DIR = Join-Path $ProjectRoot ".uv\cache"
$env:UV_LINK_MODE = "copy"
$espeakDll = "C:\Program Files\eSpeak NG\libespeak-ng.dll"
if (Test-Path $espeakDll) {
    $env:PHONEMIZER_ESPEAK_LIBRARY = $espeakDll
}

switch ($Mode) {
    "chat" {
        Write-Host "Starting Kusanagi chat..." -ForegroundColor Cyan
        uv run --project $ProjectRoot python -m kusanagi.main
    }
    "voice" {
        Write-Host "Starting Kusanagi with voice I/O..." -ForegroundColor Cyan
        uv run --project $ProjectRoot python -m kusanagi.main --voice
    }
    default {
        Write-Host "Unknown mode: $Mode. Use 'chat' or 'voice'." -ForegroundColor Red
        exit 1
    }
}
