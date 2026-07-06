param(
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot

$targets = @(
    @{Path = Join-Path $ProjectRoot ".venv"; Desc = "Virtual environment"}
    @{Path = Join-Path $ProjectRoot "uv.lock"; Desc = "uv lockfile"}
    @{Path = Join-Path $ProjectRoot "__pycache__"; Desc = "Python cache"}
    @{Path = Join-Path $ProjectRoot "src\__pycache__"; Desc = "Python cache (src)"}
    @{Path = Join-Path $ProjectRoot "*.egg-info"; Desc = "Egg info"}
)

Write-Host "=== Kusanagi Clean ===" -ForegroundColor Cyan

$totalSize = 0
foreach ($t in $targets) {
    $items = Get-ChildItem -Path $t.Path -ErrorAction SilentlyContinue
    foreach ($item in $items) {
        $size = 0
        if ($item.PSIsContainer) {
            $size = (Get-ChildItem -Path $item.FullName -Recurse -File -ErrorAction SilentlyContinue |
                     Measure-Object -Property Length -Sum).Sum
        } else {
            $size = $item.Length
        }
        $totalSize += $size
        Write-Host "  $($t.Desc): $($item.Name) ($([math]::Round($size/1MB, 2)) MB)" -ForegroundColor Gray
        if (-not $DryRun) {
            Remove-Item -Path $item.FullName -Recurse -Force -ErrorAction SilentlyContinue
            Write-Host "    removed" -ForegroundColor Yellow
        }
    }
}

Write-Host "Total reclaimed: $([math]::Round($totalSize/1MB, 2)) MB" -ForegroundColor Green
if ($DryRun) {
    Write-Host "[DRY RUN] No files were removed." -ForegroundColor Yellow
}
