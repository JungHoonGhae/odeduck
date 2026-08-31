# opendatactl installer (Windows PowerShell)
#
#   irm https://raw.githubusercontent.com/JungHoonGhae/opendatactl/main/install.ps1 | iex
#
# Environment variables:
#   $env:OPENDATACTL_VERSION  pin a version (e.g. v0.4.0, default: latest)
#   $env:GONGCTL_VERSION      legacy variable name (compatibility)
#   $env:INSTALL_DIR      install location (default: $env:LOCALAPPDATA\opendatactl)
$ErrorActionPreference = "Stop"

$Repo = "JungHoonGhae/opendatactl"
$CurrentInstallDir = Join-Path $env:LOCALAPPDATA "opendatactl"
$LegacyInstallDir = Join-Path $env:LOCALAPPDATA "gongctl"
$InstallDir = if ($env:INSTALL_DIR) {
    $env:INSTALL_DIR
}
elseif (Test-Path $CurrentInstallDir) {
    $CurrentInstallDir
}
elseif (Test-Path $LegacyInstallDir) {
    # Upgrade in place so an existing PATH entry cannot keep resolving v0.8.
    $LegacyInstallDir
}
else {
    $CurrentInstallDir
}

$Arch = switch ((Get-CimInstance Win32_Processor).Architecture) {
    12 { "arm64" }   # ARM64
    default { "amd64" }
}

# Resolve the latest tag from the releases/latest redirect (no API rate limit).
$Version = if ($env:OPENDATACTL_VERSION) { $env:OPENDATACTL_VERSION } else { $env:GONGCTL_VERSION }
if (-not $Version) {
    $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
    $Version = ($resp.Headers.Location | Select-Object -First 1) -replace ".*/tag/", ""
}
if (-not $Version) { throw "Could not resolve latest version." }
$VerNoV = $Version.TrimStart("v")

$Asset = "opendatactl_${VerNoV}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"

Write-Host "Installing opendatactl $Version ($Arch)..."
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    $Zip = Join-Path $Tmp $Asset
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Zip
    }
    catch {
        # Pinned pre-rename releases only contain gongctl_* archives.
        $Asset = "gongctl_${VerNoV}_windows_${Arch}.zip"
        $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
        $Zip = Join-Path $Tmp $Asset
        Invoke-WebRequest -Uri $Url -OutFile $Zip
    }

    # Verify against checksums.txt from the same release.
    $ChecksumFile = Join-Path $Tmp "checksums.txt"
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" -OutFile $ChecksumFile
    $Expected = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($Asset))).Line.Split(" ")[0]
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum mismatch for $Asset" }

    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    $PrimaryBinary = Join-Path $Tmp "opendatactl.exe"
    $LegacyBinary = Join-Path $Tmp "gongctl.exe"
    if (-not (Test-Path $PrimaryBinary) -and (Test-Path $LegacyBinary)) {
        Copy-Item -Path $LegacyBinary -Destination $PrimaryBinary
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Move-Item -Path $PrimaryBinary -Destination (Join-Path $InstallDir "opendatactl.exe") -Force
    if (Test-Path $LegacyBinary) {
        Move-Item -Path $LegacyBinary -Destination (Join-Path $InstallDir "gongctl.exe") -Force
    }
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

# Put the selected directory first. This also fixes existing installs where the
# legacy directory appeared before a newly appended opendatactl directory.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$NormalizedInstallDir = $InstallDir.TrimEnd("\")
$OtherPathEntries = @($UserPath -split ";" | Where-Object {
    $_ -and $_.Trim().TrimEnd("\") -ine $NormalizedInstallDir
})
$NewUserPath = (@($InstallDir) + $OtherPathEntries) -join ";"
if ($NewUserPath -ne $UserPath) {
    [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    Write-Host "Placed $InstallDir first in your user PATH. Restart the terminal to use 'opendatactl'."
}

Write-Host ""
Write-Host "Installed to $InstallDir\opendatactl.exe"
Write-Host "Compatibility alias: $InstallDir\gongctl.exe"
Write-Host "Next: opendatactl login"
