$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$Installer = Join-Path $Root "install.ps1"
$TestRoot = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
$FixtureRoot = Join-Path $TestRoot "fixtures"
$InstallRoot = Join-Path $TestRoot "install"
$OriginalPath = $env:PATH
$OriginalInstallDir = $env:INSTALL_DIR
$OriginalVersion = $env:ODEDUCK_VERSION
$OriginalUserPath = [Environment]::GetEnvironmentVariable("Path", "User")

function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)

    if ($Uri -match '/releases/latest$') {
        return [PSCustomObject]@{
            BaseResponse = [PSCustomObject]@{
                RequestMessage = [PSCustomObject]@{
                    RequestUri = [System.Uri]"https://github.com/JungHoonGhae/odeduck/releases/tag/v9.9.9"
                }
            }
        }
    }

    $Asset = ([System.Uri]$Uri).Segments[-1]
    if ($Asset -eq "odeduck-catalog.json.gz") {
        throw "fixture not found: $Asset"
    }
    $Source = Join-Path $FixtureRoot $Asset
    if (-not (Test-Path $Source)) { throw "fixture not found: $Asset" }
    Copy-Item -Path $Source -Destination $OutFile
}

try {
    New-Item -ItemType Directory -Path $FixtureRoot, $InstallRoot -Force | Out-Null
    $Binary = Join-Path $FixtureRoot "odeduck.exe"
    & go build -o $Binary ./cmd/odeduck
    if ($LASTEXITCODE -ne 0) { throw "failed to build Windows test binary" }

    $Archive = Join-Path $FixtureRoot "odeduck_9.9.9_windows_amd64.zip"
    Compress-Archive -Path $Binary -DestinationPath $Archive
    $Hash = (Get-FileHash -Algorithm SHA256 -Path $Archive).Hash.ToLower()
    Set-Content -Path (Join-Path $FixtureRoot "checksums.txt") `
        -Value "$Hash  odeduck_9.9.9_windows_amd64.zip" -Encoding utf8

    # Force the public HTTPS path even on GitHub-hosted runners where gh exists.
    $env:PATH = ""
    $env:INSTALL_DIR = $InstallRoot
    $env:ODEDUCK_VERSION = $null
    & $Installer

    if (-not (Test-Path (Join-Path $InstallRoot "odeduck.exe"))) {
        throw "installer did not create odeduck.exe"
    }
    $VersionOutput = & (Join-Path $InstallRoot "odeduck.exe") version
    if ($LASTEXITCODE -ne 0 -or $VersionOutput -notmatch '^odeduck ') {
        throw "installed binary did not run"
    }

    $MismatchRoot = Join-Path $TestRoot "mismatch"
    New-Item -ItemType Directory -Path $MismatchRoot -Force | Out-Null
    Set-Content -Path (Join-Path $FixtureRoot "checksums.txt") `
        -Value "$('0' * 64)  odeduck_9.9.9_windows_amd64.zip" -Encoding utf8
    $env:INSTALL_DIR = $MismatchRoot
    $Rejected = $false
    try {
        & $Installer
    }
    catch {
        if ($_.Exception.Message -match 'Checksum mismatch') { $Rejected = $true }
    }
    if (-not $Rejected) { throw "checksum mismatch was accepted" }
    if (Test-Path (Join-Path $MismatchRoot "odeduck.exe")) {
        throw "installer wrote a binary after checksum failure"
    }

    Write-Host "PowerShell installer tests passed"
}
finally {
    $env:PATH = $OriginalPath
    $env:INSTALL_DIR = $OriginalInstallDir
    $env:ODEDUCK_VERSION = $OriginalVersion
    [Environment]::SetEnvironmentVariable("Path", $OriginalUserPath, "User")
    Remove-Item -Recurse -Force $TestRoot -ErrorAction SilentlyContinue
}
