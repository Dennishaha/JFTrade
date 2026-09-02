$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

foreach ($command in @("cargo", "rustup", "node", "pnpm")) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "$command is not installed or not on PATH"
    }
}

if ([string]::IsNullOrWhiteSpace($env:JFTRADE_DESKTOP_RELEASE_TAG)) {
    throw "Set JFTRADE_DESKTOP_RELEASE_TAG=vX.Y.Z before building a release."
}

Write-Host "Building the Rust/Tauri desktop release for the current host..." -ForegroundColor Cyan
Write-Host "Cross-platform release artifacts are produced by the Tauri CI matrix."
pnpm install --frozen-lockfile
if ($LASTEXITCODE -ne 0) {
    throw "pnpm install failed"
}
pnpm run check:zero-go
if ($LASTEXITCODE -ne 0) {
    throw "zero-Go release gate failed"
}
pnpm run build:desktop
if ($LASTEXITCODE -ne 0) {
    throw "Tauri desktop build failed"
}

Write-Host "Tauri release artifacts are under apps/desktop/src-tauri/target/release/bundle." -ForegroundColor Green
