# ---- Step 0: Validate ----
$ErrorActionPreference = "Stop"
$cleanupPaths = @()

# ---- Step 1: Read version ----
$version = (Get-Content VERSION -Raw).Trim()
if (-not $version) {
    Write-Error "VERSION file is empty"
    exit 1
}
$tag = "v$version"
Write-Host "=== Kusanagi Release: $tag ==="

# ---- Step 2: Read changelog section for commit body ----
$changelog = Get-Content docs/meta/CHANGELOG.md -Raw
$pattern = "(?ms)## \[$version\].*?(?=\n## \[|\z)"
$commitBody = ""
if ($changelog -match $pattern) {
    $commitBody = $Matches[0].Trim()
}
$commitMsg = "release: $tag"
if ($commitBody) {
    $commitMsg += "`n`n$commitBody"
}
$msgFile = "$env:TEMP\kusanagi-commit-$(Get-Random).txt"
$commitMsg | Set-Content -Path $msgFile -Encoding UTF8
$cleanupPaths += $msgFile

# ---- Step 3: Commit and tag ----
Write-Host "Committing..."
git add -A
git commit -F $msgFile
if ($LASTEXITCODE) { throw "git commit failed" }

Write-Host "Tagging $tag..."
git tag $tag

# ---- Step 4: Push ----
Write-Host "Pushing commits..."
git push
if ($LASTEXITCODE) { throw "git push failed" }

Write-Host "Pushing tag $tag..."
git push origin $tag
if ($LASTEXITCODE) { throw "git push tag failed" }

# ---- Step 5: Wait for release workflow ----
Write-Host "Waiting for release workflow to finish..."
$runId = $null
$maxWait = 900
$elapsed = 0
$since = (Get-Date).ToUniversalTime().AddSeconds(-30)
while ($elapsed -lt $maxWait) {
    $runsJson = gh run list --workflow=Release --limit 5 --json databaseId,status,headBranch,conclusion,createdAt 2>$null
    $run = ($runsJson | ConvertFrom-Json) | Where-Object { $_.headBranch -eq $tag -and [DateTime]$_.createdAt -ge $since } | Sort-Object createdAt -Descending | Select-Object -First 1
    if ($run) {
        if ($run.status -eq "completed") {
            $runId = $run.databaseId
            if ($run.conclusion -ne "success") {
                throw "Release workflow failed: $($run.conclusion)"
            }
            break
        }
        Write-Host "  workflow running... ($($elapsed)s)"
    } else {
        Write-Host "  waiting for trigger... ($($elapsed)s)"
    }
    Start-Sleep -Seconds 15
    $elapsed += 15
}
if (-not $runId) {
    throw "Release workflow did not complete within ${maxWait}s"
}
Write-Host "Release workflow completed successfully."

# ---- Step 6: Download release asset ----
# NOTE: Once we have a proper installer, this step should download
# into $env:ProgramFiles\Kusanagi\ instead of a temp directory.
# Config store: %AppData%\Kusanagi\config.json auto-generated on
# first run (or regenerated if missing) with a terminal prompt
# telling the user to edit it with their API keys.
Write-Host "Downloading kusanagi.exe from release $tag..."
$dlDir = "$env:TEMP\kusanagi-release-$(Get-Random)"
New-Item -ItemType Directory -Path $dlDir -Force | Out-Null
$cleanupPaths += $dlDir

$dlAttempt = 0
do {
    gh release download $tag --pattern "kusanagi.exe" --dir $dlDir 2>$null
    if ($LASTEXITCODE -and $dlAttempt -lt 6) {
        Write-Host "  release not ready yet, retrying in 10s... (attempt $($dlAttempt+1))"
        Start-Sleep -Seconds 10
        $dlAttempt++
    }
} while ($LASTEXITCODE -and $dlAttempt -lt 6)
if ($LASTEXITCODE) { throw "Failed to download kusanagi.exe after 6 attempts" }

$assetSize = (Get-Item "$dlDir\kusanagi.exe").Length
$mib = [math]::Round($assetSize / 1048576, 1)

# ---- Step 7: Cleanup ----
foreach ($f in $cleanupPaths) {
    Remove-Item $f -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "=== Done ==="
Write-Host "Kusanagi v$version released: https://github.com/coff33ninja/kusanagi/releases/tag/$tag"
Write-Host "Binary: $mib MB"
