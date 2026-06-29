chcp 65001 > $null
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$embedDir = Join-Path $PSScriptRoot "internal/frontendassets/dist"
$embedArchive = Join-Path $PSScriptRoot "internal/frontendassets/dist.zip"
$webDistDir = Join-Path $PSScriptRoot "apps/web/dist"
$outputDir = Join-Path $PSScriptRoot "dist"
$buildTarget = "./cmd/jftrade-api"
$artifactPrefix = "jftrade"
$targets = @(
    @{ GOOS = "darwin"; GOARCH = "arm64" },
    @{ GOOS = "linux"; GOARCH = "amd64" },
    @{ GOOS = "windows"; GOARCH = "amd64" },
    @{ GOOS = "windows"; GOARCH = "arm64" }
)

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is not installed or not on PATH"
    }
}

function Resolve-GitValue {
    param(
        [string[]]$Arguments,
        [string]$Fallback
    )

    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        return $Fallback
    }

    try {
        $result = & git @Arguments 2>$null
        if ($LASTEXITCODE -eq 0 -and $null -ne $result) {
            $value = ($result | Select-Object -First 1).ToString().Trim()
            if ($value -ne "") {
                return $value
            }
        }
    }
    catch {
    }

    return $Fallback
}

function Install-FrontendDependencies {
    if ($env:JFTRADE_RELEASE_SKIP_NPM_INSTALL -eq "1") {
        Write-Host "Skipping npm install because JFTRADE_RELEASE_SKIP_NPM_INSTALL=1" -ForegroundColor Yellow
        return
    }

    if (Test-FrontendDependencies) {
        Write-Host "Frontend dependencies already available; skipping npm install." -ForegroundColor Green
        return
    }

    $nodeModulesDir = Join-Path $PSScriptRoot "node_modules"
    $useCleanInstall = $env:JFTRADE_RELEASE_NPM_CI -eq "1" -or -not (Test-Path $nodeModulesDir)

    if ($useCleanInstall -and (Test-Path (Join-Path $PSScriptRoot "package-lock.json"))) {
        npm ci
        if ($LASTEXITCODE -ne 0) {
            if (Test-FrontendDependencies) {
                Write-Host "npm ci failed, but existing workspace dependencies are usable; continuing." -ForegroundColor Yellow
                return
            }
            throw "npm ci failed. On Windows this is often caused by a locked native package in node_modules. Close running Node/Vite/VitePress processes, editors or antivirus scanners that may hold node_modules, then retry. If dependencies are already installed, rerun with JFTRADE_RELEASE_SKIP_NPM_INSTALL=1. To avoid clean deletion on a warm workspace, leave JFTRADE_RELEASE_NPM_CI unset."
        }
        return
    }

    npm install --workspaces --include-workspace-root --no-audit --no-fund
    if ($LASTEXITCODE -ne 0) {
        if (Test-FrontendDependencies) {
            Write-Host "npm install failed, but existing workspace dependencies are usable; continuing." -ForegroundColor Yellow
            return
        }
        throw "npm install failed. Close running Node/Vite/VitePress processes, editors or antivirus scanners that may hold node_modules, then retry."
    }
}

function Test-FrontendDependencies {
    $checks = @(
        @{ Package = "apps/web/package.json"; Module = "vite" },
        @{ Package = "apps/web/package.json"; Module = "vitepress" },
        @{ Package = "apps/web/package.json"; Module = "typedoc" },
        @{ Package = "workers/pineworker/package.json"; Module = "vite" },
        @{ Package = "workers/pineworker/package.json"; Module = "pinets" },
        @{ Package = "workers/pineworker/package.json"; Module = "@grpc/grpc-js" },
        @{ Package = "workers/pineworker/package.json"; Module = "@grpc/proto-loader" }
    )
    $resolveScript = "const { createRequire } = require('node:module'); createRequire(process.argv[1]).resolve(process.argv[2]);"

    foreach ($check in $checks) {
        $packagePath = Join-Path $PSScriptRoot $check.Package
        node -e $resolveScript $packagePath $check.Module 2>$null 1>$null
        if ($LASTEXITCODE -ne 0) {
            return $false
        }
    }

    return $true
}

function Invoke-GoReleaseBuild {
    param(
        [string]$Goos,
        [string]$Goarch,
        [string]$Version,
        [string]$Commit,
        [string]$BuildTime,
        [string]$BuildTarget,
        [string]$ArtifactPrefix
    )

    $artifactDir = Join-Path $outputDir ("{0}-{1}-{2}-{3}" -f $ArtifactPrefix, $Version, $Goos, $Goarch)
    New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null

    $outputName = if ($Goos -eq "windows") { "{0}.exe" -f $ArtifactPrefix } else { $ArtifactPrefix }
    $outputPath = Join-Path $artifactDir $outputName

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED

    try {
        $env:GOOS = $Goos
        $env:GOARCH = $Goarch
        $env:CGO_ENABLED = "0"
        $buildTags = "release_assets,netgo,osusergo"

        $ldflags = @(
            "-s",
            "-w",
            "-X github.com/jftrade/jftrade-main/internal/buildinfo.Version=$Version",
            "-X github.com/jftrade/jftrade-main/internal/buildinfo.Commit=$Commit",
            "-X github.com/jftrade/jftrade-main/internal/buildinfo.BuildTime=$BuildTime"
        ) -join " "

        Write-Host (("Building api-only {0}/{1}..." -f $Goos, $Goarch)) -ForegroundColor Green
        go build -trimpath -buildvcs=false -tags $buildTags -ldflags $ldflags -o $outputPath $BuildTarget
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $Goos/$Goarch"
        }
    }
    finally {
        if ($null -eq $previousGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoos }
        if ($null -eq $previousGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoarch }
        if ($null -eq $previousCgo) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $previousCgo }
    }
}

Require-Command go
Require-Command npm

$version = if ([string]::IsNullOrWhiteSpace($env:JFTRADE_VERSION)) {
    Resolve-GitValue -Arguments @("describe", "--tags", "--always", "--dirty") -Fallback "dev"
} else {
    $env:JFTRADE_VERSION.Trim()
}
$commit = if ([string]::IsNullOrWhiteSpace($env:JFTRADE_COMMIT)) {
    Resolve-GitValue -Arguments @("rev-parse", "--short", "HEAD") -Fallback "unknown"
} else {
    $env:JFTRADE_COMMIT.Trim()
}
$buildTime = if ([string]::IsNullOrWhiteSpace($env:JFTRADE_BUILD_TIME)) {
    [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
} else {
    $env:JFTRADE_BUILD_TIME.Trim()
}

Write-Host "Installing frontend dependencies..." -ForegroundColor Cyan
Install-FrontendDependencies

Write-Host "Building frontend bundle..." -ForegroundColor Cyan
npm run build:web
if ($LASTEXITCODE -ne 0) {
    throw "frontend build failed"
}

Write-Host "Building documentation bundle..." -ForegroundColor Cyan
npm run build:docs
if ($LASTEXITCODE -ne 0) {
    throw "documentation build failed"
}
npm run stage:docs
if ($LASTEXITCODE -ne 0) {
    throw "documentation staging failed"
}

Write-Host "Staging embedded frontend assets..." -ForegroundColor Cyan
if (Test-Path $embedDir) {
    Remove-Item -LiteralPath $embedDir -Recurse -Force
}
if (Test-Path $embedArchive) {
    Remove-Item -LiteralPath $embedArchive -Force
}
if (Test-Path $outputDir) {
    Remove-Item -LiteralPath $outputDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $embedDir) | Out-Null
New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
Copy-Item -LiteralPath $webDistDir -Destination $embedDir -Recurse
go run ./scripts/archive_frontend_assets.go -src $webDistDir -dst $embedArchive
if ($LASTEXITCODE -ne 0) {
    throw "frontend asset archiving failed"
}

Write-Host "Building embedded PineTS worker assets..." -ForegroundColor Cyan
npm run build:pineworker
if ($LASTEXITCODE -ne 0) {
    throw "PineTS worker asset build failed"
}

foreach ($target in $targets) {
    Invoke-GoReleaseBuild -Goos $target.GOOS -Goarch $target.GOARCH -Version $version -Commit $commit -BuildTime $buildTime -BuildTarget $buildTarget -ArtifactPrefix $artifactPrefix
}

Write-Host (("Release artifacts written to {0}" -f $outputDir)) -ForegroundColor Green
