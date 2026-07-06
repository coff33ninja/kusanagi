#!/usr/bin/env pwsh
param(
    [string]$Mode = "chat"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSCommandPath

Write-Host "=== Kusanagi (Go) ===" -ForegroundColor Cyan

& (Join-Path $ProjectRoot "scripts\download-servers.ps1")
if (-not $?) { exit 1 }

& (Join-Path $ProjectRoot "scripts\go-run.ps1") -Mode $Mode
