param(
    [string]$Dev = ""
)

$ErrorActionPreference = "Stop"

if ($Dev -eq "true") {
    exit 0
}

$prepared = [Environment]::GetEnvironmentVariable("JFTRADE_DESKTOP_PREPARED")
if ($prepared -ne "1") {
    Write-Error "Release assets are not prepared. Run pnpm run prepare:desktop-release first, then set JFTRADE_DESKTOP_PREPARED=1."
    exit 1
}

& node scripts/prepare-desktop-release.mjs
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
