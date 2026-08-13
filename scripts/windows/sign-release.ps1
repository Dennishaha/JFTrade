param(
    [Parameter(Mandatory = $true)]
    [string]$Binary,

    [Parameter(Mandatory = $true)]
    [string]$Installer
)

$ErrorActionPreference = "Stop"

$certificate = [Environment]::GetEnvironmentVariable("JFTRADE_WINDOWS_CERTIFICATE")
$password = [Environment]::GetEnvironmentVariable("JFTRADE_WINDOWS_CERTIFICATE_PASSWORD")
if ([string]::IsNullOrWhiteSpace($certificate) -xor [string]::IsNullOrWhiteSpace($password)) {
    throw "Windows signing credentials must be all set or all unset"
}

if ([string]::IsNullOrWhiteSpace($certificate)) {
    exit 0
}

& go tool wails3 tool sign --input $Binary --certificate $certificate --password $password --timestamp http://timestamp.digicert.com
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

& go tool wails3 tool sign --input $Installer --certificate $certificate --password $password --timestamp http://timestamp.digicert.com
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
