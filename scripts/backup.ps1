param(
    [Parameter(Mandatory)][string]$RepoPath,
    [string]$BackupDir = "backups",
    [string[]]$Exclude = @("node_modules","dist","build",".next","coverage",".git","backups"),
    [switch]$DryRun
)

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$repoName = Split-Path -Leaf (Resolve-Path $RepoPath)
$backupName = "$repoName-$timestamp.zip"
$backupPath = Join-Path $RepoPath $BackupDir $backupName

if ($DryRun) {
    Write-Host "[DRY-RUN] Would create: $backupPath"
    Write-Host "[DRY-RUN] Excluding: $($Exclude -join ', ')"
    exit 0
}

New-Item -ItemType Directory -Path (Join-Path $RepoPath $BackupDir) -Force | Out-Null

$compressParams = @{
    Path             = @((Get-ChildItem -LiteralPath $RepoPath | Where-Object {
        $_.Name -notin $Exclude
    }).FullName)
    DestinationPath  = $backupPath
    CompressionLevel = "Optimal"
}

Compress-Archive @compressParams

Write-Host "[+] Backup created: $backupPath"
Write-Host "[+] Size: $([math]::Round((Get-Item $backupPath).Length / 1MB, 2)) MB"
