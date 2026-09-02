chcp 65001 > $null
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

foreach ($command in @("cargo", "rustup", "pnpm")) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "$command is not installed or not on PATH"
    }
}

Write-Host "Starting JFTrade Tauri development desktop / 启动 JFTrade Tauri 开发桌面..." -ForegroundColor Green
Write-Host "The Rust API is the only product API entry; Go is retained for reference and contract generation only."
pnpm install --frozen-lockfile
if ($LASTEXITCODE -ne 0) {
    throw "pnpm install failed"
}
pnpm run dev:desktop
exit $LASTEXITCODE
