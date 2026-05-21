chcp 65001 > $null
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

function Join-CharCodes {
    param([int[]]$Codes)
    return -join ($Codes | ForEach-Object { [char]$_ })
}

$cnGoNotInstalled = Join-CharCodes 0x672a,0x5b89,0x88c5,0x6216,0x4e0d,0x5728,0x20,0x50,0x41,0x54,0x48,0x20,0x4e2d
$cnNpmNotInstalled = Join-CharCodes 0x672a,0x5b89,0x88c5,0x6216,0x4e0d,0x5728,0x20,0x50,0x41,0x54,0x48,0x20,0x4e2d
$cnInstallFrontend = Join-CharCodes 0x5b89,0x88c5,0x524d,0x7aef,0x4f9d,0x8d56
$cnDependencyFailed = Join-CharCodes 0x4f9d,0x8d56,0x5b89,0x88c5,0x5931,0x8d25
$cnRunGoTests = Join-CharCodes 0x8fd0,0x884c,0x20,0x47,0x6f,0x20,0x6d4b,0x8bd5
$cnGoTestsFailed = Join-CharCodes 0x47,0x6f,0x20,0x6d4b,0x8bd5,0x5931,0x8d25
$cnRunTypecheck = Join-CharCodes 0x8fd0,0x884c,0x524d,0x7aef,0x7c7b,0x578b,0x68c0,0x67e5
$cnTypecheckFailed = Join-CharCodes 0x7c7b,0x578b,0x68c0,0x67e5,0x5931,0x8d25
$cnBuildFrontend = Join-CharCodes 0x6784,0x5efa,0x524d,0x7aef
$cnFrontendBuildFailed = Join-CharCodes 0x524d,0x7aef,0x6784,0x5efa,0x5931,0x8d25
$cnStartBackend = Join-CharCodes 0x542f,0x52a8,0x540e,0x7aef,0x670d,0x52a1
$cnBackendBuildFailed = Join-CharCodes 0x540e,0x7aef,0x6784,0x5efa,0x5931,0x8d25
$cnBackendAddress = Join-CharCodes 0x540e,0x7aef,0x5730,0x5740
$cnFrontendPreview = Join-CharCodes 0x524d,0x7aef,0x9884,0x89c8
$cnStopAllServices = Join-CharCodes 0x6309,0x20,0x43,0x74,0x72,0x6c,0x2b,0x43,0x20,0x7ec8,0x6b62,0x6240,0x6709,0x670d,0x52a1
$cnStoppingBackend = Join-CharCodes 0x6b63,0x5728,0x505c,0x6b62,0x540e,0x7aef,0x670d,0x52a1
$cnBackendStopped = Join-CharCodes 0x540e,0x7aef,0x670d,0x52a1,0x5df2,0x505c,0x6b62
$cnAllExited = Join-CharCodes 0x6240,0x6709,0x670d,0x52a1,0x5df2,0x9000,0x51fa

$env:JFTRADE_API_BIND = if ([string]::IsNullOrEmpty($env:JFTRADE_API_BIND)) { "127.0.0.1:3000" }
$env:JFTRADE_FUTU_API_PORT = if ([string]::IsNullOrEmpty($env:JFTRADE_FUTU_API_PORT)) { "11110" }
$env:JFTRADE_FUTU_WEBSOCKET_PORT = if ([string]::IsNullOrEmpty($env:JFTRADE_FUTU_WEBSOCKET_PORT)) { "11111" }
$env:FUTU_OPEND_ADDR = if ([string]::IsNullOrEmpty($env:FUTU_OPEND_ADDR)) { "127.0.0.1:$($env:JFTRADE_FUTU_API_PORT)" }
$env:DISABLE_MARKETS_CACHE = if ([string]::IsNullOrEmpty($env:DISABLE_MARKETS_CACHE)) { "1" }
$env:NODE_OPTIONS = if ([string]::IsNullOrEmpty($env:NODE_OPTIONS)) { "--no-deprecation" }

if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host ("go is not installed or not on PATH / {0}" -f $cnGoNotInstalled) -ForegroundColor Red
    pause
    exit 1
}

if (-not (Get-Command "npm" -ErrorAction SilentlyContinue)) {
    Write-Host ("npm is not installed or not on PATH / {0}" -f $cnNpmNotInstalled) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Installing frontend dependencies / {0} ===" -f $cnInstallFrontend) -ForegroundColor Cyan
npm install
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Dependency installation failed / {0}" -f $cnDependencyFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Running Go tests / {0} ===" -f $cnRunGoTests) -ForegroundColor Cyan
go test ./...
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Go tests failed / {0}" -f $cnGoTestsFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Running frontend typecheck / {0} ===" -f $cnRunTypecheck) -ForegroundColor Cyan
npm run typecheck
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Typecheck failed / {0}" -f $cnTypecheckFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Building frontend / {0} ===" -f $cnBuildFrontend) -ForegroundColor Cyan
npm run build:web
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Frontend build failed / {0}" -f $cnFrontendBuildFailed) -ForegroundColor Red
    pause
    exit 1
}

Write-Host ("`n=== Starting backend service / {0} ===" -f $cnStartBackend) -ForegroundColor Green
$backendExe = Join-Path $PSScriptRoot "var\jftrade-api\jftrade-api-test.exe"
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $backendExe) | Out-Null

go build -o $backendExe ./cmd/jftrade
if ($LASTEXITCODE -ne 0) {
    Write-Host ("Backend build failed / {0}" -f $cnBackendBuildFailed) -ForegroundColor Red
    pause
    exit 1
}

$backendProcess = Start-Process -FilePath $backendExe -ArgumentList @("api") -WorkingDirectory $PSScriptRoot -PassThru
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
$watchdogProcess = Start-Process -FilePath "powershell.exe" -WindowStyle Minimized -PassThru -ArgumentList @(
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $watchdogPath,
    $PID,
    $backendProcess.Id
)

Write-Host ("JFTrade API / {0}: http://$($env:JFTRADE_API_BIND)" -f $cnBackendAddress) -ForegroundColor Green
Write-Host ("`n=== Press Ctrl+C to stop all services / {0} ===" -f $cnStopAllServices) -ForegroundColor Yellow

try {
    npm --workspace @jftrade/web run preview
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