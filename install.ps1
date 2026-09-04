# odeduck installer (Windows PowerShell)
#
#   irm https://github.com/JungHoonGhae/odeduck/releases/download/v0.17.1/install.ps1 | iex
#
# Environment variables:
#   $env:ODEDUCK_VERSION  pin a version (e.g. v0.17.1, default: latest)
#   $env:INSTALL_DIR      install location (default: $env:LOCALAPPDATA\odeduck)
$ErrorActionPreference = "Stop"

$Repo = "JungHoonGhae/odeduck"
$UseGh = $false
if (Get-Command gh -ErrorAction SilentlyContinue) {
    & gh auth status 2>$null | Out-Null
    $UseGh = $LASTEXITCODE -eq 0
}

function Get-LatestReleaseVersion {
    param([string]$Repository)

    if ($UseGh) {
        try {
            $Response = (& gh release view --repo $Repository --json tagName 2>$null) | ConvertFrom-Json
            if ($LASTEXITCODE -eq 0 -and $Response.tagName) { return $Response.tagName }
        }
        catch {
            # A public release remains available over HTTPS when gh is misconfigured.
        }
    }
    # Follow the public release redirect instead of consuming the unauthenticated
    # REST API quota shared by everyone behind the same NAT address.
    $Response = Invoke-WebRequest -Uri "https://github.com/$Repository/releases/latest" -UseBasicParsing
    $FinalUri = $Response.BaseResponse.RequestMessage.RequestUri
    if (-not $FinalUri) { $FinalUri = $Response.BaseResponse.ResponseUri }
    if (-not $FinalUri) { throw "Could not resolve the latest release redirect." }
    return ([System.Uri]$FinalUri).Segments[-1].TrimEnd("/")
}

function Receive-ReleaseAsset {
    param(
        [string]$Repository,
        [string]$Version,
        [string]$Asset,
        [string]$Destination
    )

    if ($UseGh) {
        try {
            & gh release download $Version --repo $Repository --pattern $Asset `
                --dir (Split-Path -Parent $Destination) --clobber 2>$null
            if ($LASTEXITCODE -eq 0 -and (Test-Path $Destination)) { return }
        }
        catch {
            # Fall through to the public release URL.
        }
    }
    Invoke-WebRequest -Uri "https://github.com/$Repository/releases/download/$Version/$Asset" `
        -OutFile $Destination
}

$CurrentInstallDir = Join-Path $env:LOCALAPPDATA "odeduck"
$InstallDir = if ($env:INSTALL_DIR) {
    $env:INSTALL_DIR
}
else {
    $CurrentInstallDir
}

$Arch = switch ((Get-CimInstance Win32_Processor).Architecture) {
    12 { "arm64" }   # ARM64
    default { "amd64" }
}

$Version = if ($env:ODEDUCK_VERSION) {
    $env:ODEDUCK_VERSION
}
if (-not $Version) {
    $Version = Get-LatestReleaseVersion -Repository $Repo
}
if (-not $Version) { throw "Could not resolve latest version." }
$VerNoV = $Version.TrimStart("v")

$Asset = "odeduck_${VerNoV}_windows_${Arch}.zip"

Write-Host "Installing odeduck $Version ($Arch)..."
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    $Zip = Join-Path $Tmp $Asset
    Receive-ReleaseAsset -Repository $Repo -Version $Version -Asset $Asset -Destination $Zip

    $CatalogAsset = "odeduck-catalog.json.gz"
    $CatalogFile = Join-Path $Tmp $CatalogAsset
    $CatalogAvailable = $false
    try {
        Receive-ReleaseAsset -Repository $Repo -Version $Version -Asset $CatalogAsset -Destination $CatalogFile
        $CatalogAvailable = $true
    }
    catch {
        Write-Host "Prebuilt catalogue is not available for $Version; install will continue without it."
    }

    # Verify against checksums.txt from the same release.
    $ChecksumFile = Join-Path $Tmp "checksums.txt"
    Receive-ReleaseAsset -Repository $Repo -Version $Version -Asset "checksums.txt" -Destination $ChecksumFile
    $Expected = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($Asset))).Line.Split(" ")[0]
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum mismatch for $Asset" }
    if ($CatalogAvailable) {
        $ExpectedCatalog = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($CatalogAsset))).Line.Split(" ")[0]
        $ActualCatalog = (Get-FileHash -Algorithm SHA256 -Path $CatalogFile).Hash.ToLower()
        if ($ExpectedCatalog -ne $ActualCatalog) { throw "Checksum mismatch for $CatalogAsset" }
    }

    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    $PrimaryBinary = Join-Path $Tmp "odeduck.exe"
    if (-not (Test-Path $PrimaryBinary)) { throw "odeduck.exe missing from $Asset" }
    if ($CatalogAvailable) {
        & $PrimaryBinary catalog install-snapshot $CatalogFile --check-only -f table
        if ($LASTEXITCODE -ne 0) { throw "Prebuilt catalogue validation failed" }
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Move-Item -Path $PrimaryBinary -Destination (Join-Path $InstallDir "odeduck.exe") -Force
    if ($CatalogAvailable) {
        & (Join-Path $InstallDir "odeduck.exe") catalog install-snapshot $CatalogFile -f table
        if ($LASTEXITCODE -ne 0) { throw "Prebuilt catalogue install failed" }
    }
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

# Put the selected directory first in the user PATH.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$NormalizedInstallDir = $InstallDir.TrimEnd("\")
$OtherPathEntries = @($UserPath -split ";" | Where-Object {
    $_ -and $_.Trim().TrimEnd("\") -ine $NormalizedInstallDir
})
$NewUserPath = (@($InstallDir) + $OtherPathEntries) -join ";"
if ($NewUserPath -ne $UserPath) {
    [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    Write-Host "Placed $InstallDir first in your user PATH. Restart the terminal to use 'odeduck'."
}

Write-Host ""
Write-Host "Installed to $InstallDir\odeduck.exe"
Write-Host "Next: odeduck login"
