param(
  [Parameter(Mandatory = $true)][string]$CandidateRoot,
  [Parameter(Mandatory = $true)][string]$BaselinePackage,
  [Parameter(Mandatory = $true)][string]$ReportPath
)

$ErrorActionPreference = "Stop"
$workRoot = Join-Path $env:RUNNER_TEMP "jftrade-release-rehearsal-windows-arm64"
if ($env:REHEARSAL_PLATFORM -eq "windows-x64") {
  $workRoot = Join-Path $env:RUNNER_TEMP "jftrade-release-rehearsal-windows-x64"
}
$installRoot = Join-Path $workRoot "install"
$backupRoot = Join-Path $workRoot "backup"
$runtimeHome = Join-Path $workRoot "home"
Remove-Item -Recurse -Force $workRoot -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $installRoot, $runtimeHome, (Split-Path $ReportPath) | Out-Null
$env:USERPROFILE = $runtimeHome
$env:LOCALAPPDATA = Join-Path $runtimeHome "AppData\Local"
New-Item -ItemType Directory -Force $env:LOCALAPPDATA | Out-Null

function Install-Package([string]$Installer) {
  $process = Start-Process -FilePath $Installer -ArgumentList "/S", "/D=$installRoot" -Wait -PassThru
  if ($process.ExitCode -ne 0) { throw "installer failed with exit code $($process.ExitCode): $Installer" }
}

function Find-Executable {
  $executable = Get-ChildItem $installRoot -Recurse -File -Filter "*.exe" |
    Where-Object { $_.Name -notmatch "(?i)unins|uninstall|helper|sidecar" } |
    Select-Object -First 1
  if (-not $executable) { throw "installed JFTrade executable was not found" }
  return $executable.FullName
}

function Probe-Runtime([string]$Executable, [string]$Label) {
  $stdout = Join-Path $workRoot "$Label.stdout.log"
  $stderr = Join-Path $workRoot "$Label.stderr.log"
  $process = Start-Process -FilePath $Executable -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
  try {
    $ready = $false
    for ($attempt = 0; $attempt -lt 120; $attempt++) {
      if ($process.HasExited) { throw "$Label exited before runtime readiness with $($process.ExitCode)" }
      try {
        $response = Invoke-WebRequest -Uri "http://127.0.0.1:6699/api/v1/system/status" -TimeoutSec 2 -SkipHttpErrorCheck
        if ($response.StatusCode -in @(200, 401)) { $ready = $true; break }
      } catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $ready) { throw "$Label did not become ready" }
    $body = $response.Content | ConvertFrom-Json
    if ($Label -eq "candidate-upgrade") {
      if ($response.StatusCode -ne 401 -or $body.error.code -ne "WEB_AUTH_REQUIRED") {
        throw "candidate API did not fail closed for an unauthenticated request"
      }
    }
  } finally {
    if (-not $process.HasExited) {
      $null = $process.CloseMainWindow()
      if (-not $process.WaitForExit(5000)) { Stop-Process -Id $process.Id -Force }
    }
    $process.WaitForExit()
  }
  for ($attempt = 0; $attempt -lt 40; $attempt++) {
    try {
      Invoke-WebRequest -Uri "http://127.0.0.1:6699/api/v1/system/status" -TimeoutSec 1 -SkipHttpErrorCheck | Out-Null
      Start-Sleep -Milliseconds 250
    } catch { return }
  }
  throw "$Label left the API listener running after shutdown"
}

Install-Package $BaselinePackage
$baselineExecutable = Find-Executable
Probe-Runtime $baselineExecutable "baseline-first-start"
$dataRoot = Join-Path $env:LOCALAPPDATA "JFTrade"
if (-not (Test-Path $dataRoot -PathType Container)) { throw "baseline did not create Windows data root" }
Copy-Item -Recurse -Force $dataRoot $backupRoot

$candidatePackage = Get-ChildItem $CandidateRoot -Recurse -File -Filter "*-setup.exe" | Select-Object -First 1
if (-not $candidatePackage) { throw "candidate artifact has no NSIS installer" }
Install-Package $candidatePackage.FullName
$candidateExecutable = Find-Executable
Probe-Runtime $candidateExecutable "candidate-upgrade"
$upgradeCheckRoot = Join-Path $workRoot "upgraded-data"
Copy-Item -Recurse -Force $dataRoot $upgradeCheckRoot

$uninstaller = Get-ChildItem $installRoot -Recurse -File -Filter "*.exe" |
  Where-Object { $_.Name -match "(?i)unins|uninstall" } |
  Select-Object -First 1
if (-not $uninstaller) { throw "candidate uninstaller was not found" }
$uninstall = Start-Process -FilePath $uninstaller.FullName -ArgumentList "/S" -Wait -PassThru
if ($uninstall.ExitCode -ne 0) { throw "candidate uninstall failed with $($uninstall.ExitCode)" }
if (Test-Path $installRoot) { Remove-Item -Recurse -Force $installRoot }
Remove-Item -Recurse -Force $dataRoot
Copy-Item -Recurse -Force $backupRoot $dataRoot
New-Item -ItemType Directory -Force $installRoot | Out-Null
Install-Package $BaselinePackage
$rollbackExecutable = Find-Executable
Probe-Runtime $rollbackExecutable "baseline-rollback"
$rollbackUninstaller = Get-ChildItem $installRoot -Recurse -File -Filter "*.exe" |
  Where-Object { $_.Name -match "(?i)unins|uninstall" } |
  Select-Object -First 1
if ($rollbackUninstaller) { Start-Process -FilePath $rollbackUninstaller.FullName -ArgumentList "/S" -Wait | Out-Null }

@'
import pathlib, sqlite3, sys
root = pathlib.Path(sys.argv[1])
expected = {
    "backtest.db", "backtest-runs.db", "strategy-runtime.db", "execution-orders.db",
    "adk.db", "adk-session.db", "adk-artifact.db", "watchlists.db", "research.db",
}
actual = {path.name for path in root.glob("*.db")}
if actual != expected:
    raise SystemExit(f"expected exactly nine managed databases, got {sorted(actual)}")
for name in sorted(expected):
    connection = sqlite3.connect(f"file:{root / name}?mode=ro", uri=True)
    try:
        result = connection.execute("PRAGMA integrity_check").fetchone()
        if result != ("ok",): raise SystemExit(f"{name} integrity_check failed: {result}")
    finally:
        connection.close()
'@ | python - $upgradeCheckRoot
if ($LASTEXITCODE -ne 0) { throw "nine-database integrity verification failed" }

$sbom = Get-ChildItem $CandidateRoot -Recurse -File |
  Where-Object { $_.Name -match "(?i)\.spdx\.json$|sbom.*\.json$" } |
  Select-Object -First 1
if (-not $sbom) { throw "candidate artifact has no SBOM" }
pnpm run check:zero-go -- --artifact $candidatePackage.FullName --artifact $sbom.FullName
if ($LASTEXITCODE -ne 0) { throw "zero-Go artifact check failed" }

$checks = [ordered]@{}
@("package", "install", "firstStart", "upgrade", "databaseUpgrade", "runtimeSmoke", "uninstall", "backupRestore", "rollback", "zeroGo", "sbomZeroGo") |
  ForEach-Object { $checks[$_] = "passed" }
$report = [ordered]@{
  schemaVersion = "jftrade.release-rehearsal-platform.v1"
  qualificationMode = "rehearsal"
  platform = $env:REHEARSAL_PLATFORM
  status = "passed"
  candidateRef = $env:REHEARSAL_CANDIDATE_REF
  plannedReleaseTag = $env:REHEARSAL_PLANNED_TAG
  commitSha = $env:REHEARSAL_COMMIT_SHA
  artifact = [ordered]@{
    name = $env:REHEARSAL_ARTIFACT_NAME
    id = [long]$env:REHEARSAL_ARTIFACT_ID
    digest = $env:REHEARSAL_ARTIFACT_DIGEST
  }
  checks = $checks
}
$report | ConvertTo-Json -Depth 8 | Set-Content -Path $ReportPath -Encoding utf8NoBOM -NoNewline
