# oddsock installer (Windows PowerShell)
#
#   (& gh api -H "Accept: application/vnd.github.raw+json" repos/JungHoonGhae/oddsock/contents/install.ps1) |
#     Out-String | Invoke-Expression
#
# Environment variables:
#   $env:ODDSOCK_VERSION  pin a version (e.g. v0.4.0, default: latest)
#   $env:OPENDATACTL_VERSION / $env:GONGCTL_VERSION legacy names (compatibility)
#   $env:INSTALL_DIR      install location (default: $env:LOCALAPPDATA\oddsock)
$ErrorActionPreference = "Stop"

$Repo = "JungHoonGhae/oddsock"
$CurrentInstallDir = Join-Path $env:LOCALAPPDATA "oddsock"
$FormerInstallDir = Join-Path $env:LOCALAPPDATA "opendatactl"
$LegacyInstallDir = Join-Path $env:LOCALAPPDATA "gongctl"
$InstallDir = if ($env:INSTALL_DIR) {
    $env:INSTALL_DIR
}
elseif (Test-Path $CurrentInstallDir) {
    $CurrentInstallDir
}
elseif (Test-Path $FormerInstallDir) {
    $FormerInstallDir
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

$Version = if ($env:ODDSOCK_VERSION) {
    $env:ODDSOCK_VERSION
}
elseif ($env:OPENDATACTL_VERSION) {
    $env:OPENDATACTL_VERSION
}
else {
    $env:GONGCTL_VERSION
}
if (-not $Version) {
    if (Get-Command gh -ErrorAction SilentlyContinue) {
        $Version = ((& gh release view --repo $Repo --json tagName) | ConvertFrom-Json).tagName
    }
    else {
        $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
        $Version = ($resp.Headers.Location | Select-Object -First 1) -replace ".*/tag/", ""
    }
}
if (-not $Version) { throw "Could not resolve latest version." }
$VerNoV = $Version.TrimStart("v")

$Asset = "oddsock_${VerNoV}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"

Write-Host "Installing oddsock $Version ($Arch)..."
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    $Zip = Join-Path $Tmp $Asset
    try {
        if (Get-Command gh -ErrorAction SilentlyContinue) {
            & gh release download $Version --repo $Repo --pattern $Asset --dir $Tmp --clobber
            if ($LASTEXITCODE -ne 0) { throw "gh release download failed" }
        }
        else {
            Invoke-WebRequest -Uri $Url -OutFile $Zip
        }
    }
    catch {
        try {
            $Asset = "opendatactl_${VerNoV}_windows_${Arch}.zip"
            $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
            $Zip = Join-Path $Tmp $Asset
            if (Get-Command gh -ErrorAction SilentlyContinue) {
                & gh release download $Version --repo $Repo --pattern $Asset --dir $Tmp --clobber
                if ($LASTEXITCODE -ne 0) { throw "gh release download failed" }
            }
            else {
                Invoke-WebRequest -Uri $Url -OutFile $Zip
            }
        }
        catch {
            # v0.8 and earlier only shipped gongctl_* archives.
            $Asset = "gongctl_${VerNoV}_windows_${Arch}.zip"
            $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
            $Zip = Join-Path $Tmp $Asset
            if (Get-Command gh -ErrorAction SilentlyContinue) {
                & gh release download $Version --repo $Repo --pattern $Asset --dir $Tmp --clobber
                if ($LASTEXITCODE -ne 0) { throw "gh release download failed" }
            }
            else {
                Invoke-WebRequest -Uri $Url -OutFile $Zip
            }
        }
    }

    $CatalogAsset = "oddsock-catalog.json.gz"
    $CatalogFile = Join-Path $Tmp $CatalogAsset
    $CatalogAvailable = $false
    try {
        if (Get-Command gh -ErrorAction SilentlyContinue) {
            & gh release download $Version --repo $Repo --pattern $CatalogAsset --dir $Tmp --clobber
            if ($LASTEXITCODE -ne 0) { throw "gh catalogue download failed" }
        }
        else {
            Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/$CatalogAsset" -OutFile $CatalogFile
        }
        $CatalogAvailable = $true
    }
    catch {
        $CatalogAsset = "opendatactl-catalog.json.gz"
        $CatalogFile = Join-Path $Tmp $CatalogAsset
        try {
            if (Get-Command gh -ErrorAction SilentlyContinue) {
                & gh release download $Version --repo $Repo --pattern $CatalogAsset --dir $Tmp --clobber
                if ($LASTEXITCODE -ne 0) { throw "gh catalogue download failed" }
            }
            else {
                Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/$CatalogAsset" -OutFile $CatalogFile
            }
            $CatalogAvailable = $true
        }
        catch {
            Write-Host "Prebuilt catalogue is not available for $Version; install will continue without it."
        }
    }

    # Verify against checksums.txt from the same release.
    $ChecksumFile = Join-Path $Tmp "checksums.txt"
    if (Get-Command gh -ErrorAction SilentlyContinue) {
        & gh release download $Version --repo $Repo --pattern "checksums.txt" --dir $Tmp --clobber
        if ($LASTEXITCODE -ne 0) { throw "gh checksum download failed" }
    }
    else {
        Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" -OutFile $ChecksumFile
    }
    $Expected = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($Asset))).Line.Split(" ")[0]
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum mismatch for $Asset" }
    if ($CatalogAvailable) {
        $ExpectedCatalog = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($CatalogAsset))).Line.Split(" ")[0]
        $ActualCatalog = (Get-FileHash -Algorithm SHA256 -Path $CatalogFile).Hash.ToLower()
        if ($ExpectedCatalog -ne $ActualCatalog) { throw "Checksum mismatch for $CatalogAsset" }
    }

    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    $PrimaryBinary = Join-Path $Tmp "oddsock.exe"
    $FormerBinary = Join-Path $Tmp "opendatactl.exe"
    $LegacyBinary = Join-Path $Tmp "gongctl.exe"
    if (-not (Test-Path $PrimaryBinary)) {
        if (Test-Path $FormerBinary) {
            Copy-Item -Path $FormerBinary -Destination $PrimaryBinary
        }
        elseif (Test-Path $LegacyBinary) {
            Copy-Item -Path $LegacyBinary -Destination $PrimaryBinary
        }
    }
    if (-not (Test-Path $FormerBinary)) { Copy-Item -Path $PrimaryBinary -Destination $FormerBinary }
    if (-not (Test-Path $LegacyBinary)) { Copy-Item -Path $PrimaryBinary -Destination $LegacyBinary }
    if ($CatalogAvailable) {
        & $PrimaryBinary catalog install-snapshot $CatalogFile --check-only -f table
        if ($LASTEXITCODE -ne 0) { throw "Prebuilt catalogue validation failed" }
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Move-Item -Path $PrimaryBinary -Destination (Join-Path $InstallDir "oddsock.exe") -Force
    Move-Item -Path $FormerBinary -Destination (Join-Path $InstallDir "opendatactl.exe") -Force
    Move-Item -Path $LegacyBinary -Destination (Join-Path $InstallDir "gongctl.exe") -Force
    if ($CatalogAvailable) {
        & (Join-Path $InstallDir "oddsock.exe") catalog install-snapshot $CatalogFile -f table
        if ($LASTEXITCODE -ne 0) { throw "Prebuilt catalogue install failed" }
    }
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

# Put the selected directory first. This also fixes existing installs where the
# legacy directory appeared before a newly appended oddsock directory.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$NormalizedInstallDir = $InstallDir.TrimEnd("\")
$OtherPathEntries = @($UserPath -split ";" | Where-Object {
    $_ -and $_.Trim().TrimEnd("\") -ine $NormalizedInstallDir
})
$NewUserPath = (@($InstallDir) + $OtherPathEntries) -join ";"
if ($NewUserPath -ne $UserPath) {
    [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    Write-Host "Placed $InstallDir first in your user PATH. Restart the terminal to use 'oddsock'."
}

Write-Host ""
Write-Host "Installed to $InstallDir\oddsock.exe"
Write-Host "Compatibility aliases: $InstallDir\opendatactl.exe, $InstallDir\gongctl.exe"
Write-Host "Next: oddsock login"
