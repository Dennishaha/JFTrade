chcp 65001 > $null
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$webDistDir = Join-Path $PSScriptRoot "apps\web\dist"
$embedDir = Join-Path $PSScriptRoot "internal\frontendassets\dist"
$embedArchive = Join-Path $PSScriptRoot "internal\frontendassets\dist.zip"
$runtimeDir = Join-Path $PSScriptRoot "var\jftrade-api"
$settingsPath = Join-Path $runtimeDir "settings.json"
$backtestDBPath = Join-Path $runtimeDir "backtest.db"

function Join-CharCodes {
    param([int[]]$Codes)
    return -join ($Codes | ForEach-Object { [char]$_ })
}

$cnGoNotInstalled = Join-CharCodes 0x672a,0x5b89,0x88c5,0x6216,0x4e0d,0x5728,0x20,0x50,0x41,0x54,0x48,0x20,0x4e2d
$cnPnpmNotInstalled = Join-CharCodes 0x672a,0x5b89,0x88c5,0x6216,0x4e0d,0x5728,0x20,0x50,0x41,0x54,0x48,0x20,0x4e2d
$cnInstallFrontend = Join-CharCodes 0x5b89,0x88c5,0x524d,0x7aef,0x4f9d,0x8d56
$cnDependencyFailed = Join-CharCodes 0x4f9d,0x8d56,0x5b89,0x88c5,0x5931,0x8d25
$cnRunTypecheck = Join-CharCodes 0x8fd0,0x884c,0x524d,0x7aef,0x7c7b,0x578b,0x68c0,0x67e5
$cnTypecheckFailed = Join-CharCodes 0x7c7b,0x578b,0x68c0,0x67e5,0x5931,0x8d25
$cnBuildFrontend = Join-CharCodes 0x6784,0x5efa,0x524d,0x7aef
$cnFrontendBuildFailed = Join-CharCodes 0x524d,0x7aef,0x6784,0x5efa,0x5931,0x8d25
$cnStartBackend = Join-CharCodes 0x542f,0x52a8,0x540e,0x7aef,0x670d,0x52a1
$cnBackendBuildFailed = Join-CharCodes 0x540e,0x7aef,0x6784,0x5efa,0x5931,0x8d25
$cnStopAllServices = Join-CharCodes 0x6309,0x20,0x43,0x74,0x72,0x6c,0x2b,0x43,0x20,0x7ec8,0x6b62,0x6240,0x6709,0x670d,0x52a1
$cnStoppingBackend = Join-CharCodes 0x6b63,0x5728,0x505c,0x6b62,0x540e,0x7aef,0x670d,0x52a1
$cnBackendStopped = Join-CharCodes 0x540e,0x7aef,0x670d,0x52a1,0x5df2,0x505c,0x6b62
$cnAllExited = Join-CharCodes 0x6240,0x6709,0x670d,0x52a1,0x5df2,0x9000,0x51fa

function Set-DefaultEnv {
    param(
        [string]$Name,
        [string]$Value
    )

    if ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($Name))) {
        [Environment]::SetEnvironmentVariable($Name, $Value, "Process")
    }
}

Set-DefaultEnv "JFTRADE_GUI_BIND" "127.0.0.1:6688"
Set-DefaultEnv "JFTRADE_SETTINGS_PATH" $settingsPath
Set-DefaultEnv "JFTRADE_BACKTEST_DB" $backtestDBPath
Set-DefaultEnv "JFTRADE_FUTU_API_PORT" "11110"
Set-DefaultEnv "JFTRADE_FUTU_WEBSOCKET_PORT" "11111"
Set-DefaultEnv "FUTU_OPEND_ADDR" "127.0.0.1:$($env:JFTRADE_FUTU_API_PORT)"
Set-DefaultEnv "DISABLE_MARKETS_CACHE" "1"
Set-DefaultEnv "NODE_OPTIONS" "--no-deprecation"

if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host ("go is not installed or not on PATH / {0}" -f $cnGoNotInstalled) -ForegroundColor Red
    pause
    exit 1
}

if (-not (Get-Command "pnpm" -ErrorAction SilentlyContinue)) {
    Write-Host ("pnpm is not installed or not on PATH / {0}" -f $cnPnpmNotInstalled) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Installing frontend dependencies / {0} ===" -f $cnInstallFrontend) -ForegroundColor Cyan
pnpm install --frozen-lockfile
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Dependency installation failed / {0}" -f $cnDependencyFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host "`n=== Generating Swagger docs / 生成 Swagger 文档 ===" -ForegroundColor Cyan
pnpm run generate:openapi
if ($LASTEXITCODE -ne 0) {
    Write-Host "Swagger generation failed / Swagger 文档生成失败" -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Running frontend typecheck / {0} ===" -f $cnRunTypecheck) -ForegroundColor Cyan
pnpm run typecheck
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Typecheck failed / {0}" -f $cnTypecheckFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Building frontend / {0} ===" -f $cnBuildFrontend) -ForegroundColor Cyan
pnpm run build:web
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Frontend build failed / {0}" -f $cnFrontendBuildFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host "`n=== Staging embedded frontend assets ===" -ForegroundColor Cyan
if (Test-Path $embedDir) {
    Remove-Item -LiteralPath $embedDir -Recurse -Force
}
if (Test-Path $embedArchive) {
    Remove-Item -LiteralPath $embedArchive -Force
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $embedDir) | Out-Null
Copy-Item -LiteralPath $webDistDir -Destination $embedDir -Recurse
go run ./scripts/archive_frontend_assets.go -src $webDistDir -dst $embedArchive
if ($LASTEXITCODE -ne 0) {
    Write-Host "Frontend asset archiving failed" -ForegroundColor Red
    pause
    exit 1
}

Write-Host "`n=== Building embedded PineTS worker assets ===" -ForegroundColor Cyan
pnpm run build:pineworker
if ($LASTEXITCODE -ne 0) {
    Write-Host "PineTS worker asset build failed" -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Starting backend service / {0} ===" -f $cnStartBackend) -ForegroundColor Green
$backendExe = Join-Path $PSScriptRoot "dist\jftrade-api-test.exe"
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $backendExe) | Out-Null

go build -tags release_assets -o $backendExe ./cmd/jftrade-api
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Backend build failed / {0}" -f $cnBackendBuildFailed) -ForegroundColor Red
    pause
    exit 1
}

$backendProcess = Start-Process -FilePath $backendExe -WorkingDirectory $PSScriptRoot -PassThru
$watchdogPath = Join-Path $env:TEMP ("jftrade-watchdog-{0}.ps1" -f ([guid]::NewGuid().ToString("N")))
@'
param($launcherPid, $backendPid)
try {
    Wait-Process -Id $launcherPid -ErrorAction SilentlyContinue
} finally {
    if (Get-Process -Id $backendPid -ErrorAction SilentlyContinue) {
        taskkill /PID $backendPid /T /F | Out-Null
    }
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
'@ | Set-Content -LiteralPath $watchdogPath -Encoding ASCII
$watchdogProcess = Start-Process -FilePath "powershell.exe" -WindowStyle hidden -PassThru -ArgumentList @(
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $watchdogPath,
    $PID,
    $backendProcess.Id
)

Write-Host ("Optional Web address (disabled by default): http://$($env:JFTRADE_GUI_BIND)") -ForegroundColor Green
Write-Host "Enable Web access and set its password in JFTrade Dev > Settings first / 请先在 JFTrade Dev 的设置中开启 Web 访问并设置密码" -ForegroundColor Yellow
Write-Host ("`n=== Press Ctrl+C to stop all services / {0} ===" -f $cnStopAllServices) -ForegroundColor Yellow

try {
    Wait-Process -Id $backendProcess.Id
}
finally {
    Write-Host ("`n=== Stopping backend service / {0} ===" -f $cnStoppingBackend) -ForegroundColor Cyan
    if ($backendProcess -and !$backendProcess.HasExited) {
        taskkill /PID $backendProcess.Id /T /F | Out-Null
    }
    if ($watchdogProcess -and !$watchdogProcess.HasExited) {
        Stop-Process -Id $watchdogProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if ($watchdogPath) {
        Remove-Item -LiteralPath $watchdogPath -Force -ErrorAction SilentlyContinue
    }
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.CommandLine -like "*$backendExe*"
    } | ForEach-Object {
        Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
    }

    Write-Host ("Backend service stopped / {0}" -f $cnBackendStopped) -ForegroundColor Green
    Write-Host ("All services exited / {0}`n" -f $cnAllExited) -ForegroundColor Green

    Exit-PSHostProcess
}
