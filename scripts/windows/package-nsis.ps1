param(
    [Parameter(Mandatory = $true)]
    [string]$WailsBuildDir,

    [Parameter(Mandatory = $true)]
    [string]$BinDir,

    [Parameter(Mandatory = $true)]
    [string]$AppName,

    [Parameter(Mandatory = $true)]
    [string]$Installer,

    [Parameter(Mandatory = $true)]
    [string]$ArgFlag
)

$ErrorActionPreference = "Stop"

$nsisDir = Join-Path $WailsBuildDir "windows/nsis"
New-Item -ItemType Directory -Force $nsisDir | Out-Null
Copy-Item "build/windows/nsis/project.nsi" (Join-Path $nsisDir "project.nsi") -Force

& go tool wails3 generate webview2bootstrapper -dir $nsisDir
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$binary = (Resolve-Path (Join-Path $BinDir "$AppName.exe")).Path
$installerPath = Join-Path (Resolve-Path $BinDir).Path $Installer
$license = (Resolve-Path "LICENSE").Path
$notices = (Resolve-Path "docs/legal/third-party-notices.md").Path
$makensis = (Get-Command makensis -ErrorAction Stop).Source
$arguments = @(
    "/DARG_WAILS_${ArgFlag}_BINARY=$binary"
    "/DJFTRADE_LICENSE_FILE=$license"
    "/DJFTRADE_THIRD_PARTY_NOTICES_FILE=$notices"
    "/DOUTPUT_EXE=$installerPath"
    "project.nsi"
)

Push-Location $nsisDir
try {
    & $makensis @arguments
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
