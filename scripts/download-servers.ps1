param(
    [string]$Version = "",
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$ServersDir = Join-Path $ProjectRoot "servers"
$exePath = Join-Path $ServersDir "mcp-server.exe"
$versionFile = Join-Path $ServersDir "version.txt"

Write-Host "=== Download MCP Servers ===" -ForegroundColor Cyan

if (-not (Test-Path $ServersDir)) {
    New-Item -ItemType Directory -Path $ServersDir -Force | Out-Null
}

# Determine latest version from GitHub
if (-not $Version) {
    Write-Host "Checking latest release..." -ForegroundColor Yellow
    try {
        $apiUrl = "https://api.github.com/repos/coff33ninja/go-mcp-computer-use/releases/latest"
        $latest = Invoke-RestMethod -Uri $apiUrl -Headers @{"Accept" = "application/vnd.github.v3+json"}
        $Version = $latest.tag_name
        Write-Host ("Latest release: " + $Version) -ForegroundColor Green
    } catch {
        Write-Host ("Failed to check latest release: " + $_) -ForegroundColor Red
        if (Test-Path $versionFile) {
            $Version = (Get-Content $versionFile -Raw).Trim()
            Write-Host ("Falling back to stored version: " + $Version) -ForegroundColor Yellow
        } else {
            $Version = "v0.2.31"
            Write-Host ("Falling back to hardcoded version: " + $Version) -ForegroundColor Yellow
        }
    }
}

# Compare with installed version
if (Test-Path $versionFile) {
    $currentVersion = (Get-Content $versionFile -Raw).Trim()
    if ($currentVersion -eq $Version) {
        $sizeMb = [math]::Round((Get-Item $exePath).Length / 1MB, 1)
        Write-Host ("mcp-server.exe " + $Version + " already installed (" + $sizeMb + " MB)") -ForegroundColor Green
        exit 0
    }
    Write-Host ("Updating from " + $currentVersion + " to " + $Version + "...") -ForegroundColor Yellow
} elseif (Test-Path $exePath) {
    $sizeMb = [math]::Round((Get-Item $exePath).Length / 1MB, 1)
    Write-Host ("Existing mcp-server.exe found (" + $sizeMb + " MB) -- no version file, will re-download") -ForegroundColor Yellow
}

$url = "https://github.com/coff33ninja/go-mcp-computer-use/releases/download/$Version/mcp-server.exe"
$shaUrl = "$url.sha256"

if ($DryRun) {
    Write-Host "[DRY RUN] Would download:" -ForegroundColor Yellow
    Write-Host ("  " + $url) -ForegroundColor Yellow
    Write-Host ("  -> " + $exePath) -ForegroundColor Yellow
    exit 0
}

Write-Host ("Downloading mcp-server.exe " + $Version + "...") -ForegroundColor Yellow
Write-Host ("  " + $url) -ForegroundColor Gray

$wc = New-Object System.Net.WebClient
try {
    $wc.DownloadFile($url, $exePath)
} catch {
    Write-Host ("Download failed: " + $_) -ForegroundColor Red
    exit 1
}

try {
    $shaFile = [System.IO.Path]::GetTempFileName()
    $wc.DownloadFile($shaUrl, $shaFile)
    $expected = (Get-Content $shaFile -Raw).Trim().Split(' ')[0]
    $actual = (Get-FileHash -Path $exePath -Algorithm SHA256).Hash.ToLower()
    Remove-Item -LiteralPath $shaFile -Force
    if ($expected -ne $actual) {
        Write-Host ("SHA256 mismatch! Expected: " + $expected) -ForegroundColor Red
        Write-Host ("  Actual: " + $actual) -ForegroundColor Red
        Remove-Item -LiteralPath $exePath -Force
        exit 1
    }
    Write-Host "SHA256 verified." -ForegroundColor Green
} catch {
    Write-Host "SHA256 verification skipped (no checksum file)." -ForegroundColor Yellow
}

# Save version
Set-Content -Path $versionFile -Value $Version -NoNewline

$sizeMb = [math]::Round((Get-Item $exePath).Length / 1MB, 1)
Write-Host ("Downloaded: " + $exePath + " (" + $sizeMb + " MB)") -ForegroundColor Green
Write-Host "=== Done ===" -ForegroundColor Cyan
