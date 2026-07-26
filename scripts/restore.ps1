param(
    [Parameter(Mandatory = $true)][string]$ArchivePath,
    [string]$DatabasePath = ".data\zaiko.db",
    [string]$UploadDirectory = ".data\uploads",
    [string]$MaintenanceBinary = "bin\zaiko-maintenance.exe",
    [Parameter(Mandatory = $true)][ValidateSet("RESTORE")][string]$Confirm
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$archiveAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $ArchivePath))
$databaseAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $DatabasePath))
$uploadsAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $UploadDirectory))
$maintenanceAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $MaintenanceBinary))
if (-not (Test-Path -LiteralPath $archiveAbsolute -PathType Leaf)) { throw "Backup archive not found." }
if (-not (Test-Path -LiteralPath $maintenanceAbsolute -PathType Leaf)) { throw "Maintenance tool not found." }
if ((Split-Path -Parent $databaseAbsolute) -eq [System.IO.Path]::GetPathRoot($databaseAbsolute)) {
    throw "The database cannot be restored directly under a drive root."
}
if ($uploadsAbsolute -eq [System.IO.Path]::GetPathRoot($uploadsAbsolute)) {
    throw "The uploads directory cannot be a drive root."
}
$running = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like "zaiko-kanri*" }
if ($running) { throw "Stop the inventory server before restore." }
if ((Test-Path -LiteralPath "$databaseAbsolute-wal") -or (Test-Path -LiteralPath "$databaseAbsolute-shm")) {
    throw "SQLite WAL files remain. Confirm a clean shutdown and checkpoint."
}

$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("zaiko-restore-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    Expand-Archive -LiteralPath $archiveAbsolute -DestinationPath $temporary
    $manifestPath = Join-Path $temporary "manifest.json"
    $restoredDatabase = Join-Path $temporary "database\zaiko.db"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf) -or -not (Test-Path -LiteralPath $restoredDatabase -PathType Leaf)) {
        throw "Invalid backup contents."
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $actualHash = (Get-FileHash -LiteralPath $restoredDatabase -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne [string]$manifest.database_sha256) { throw "Backup hash mismatch." }
    & $maintenanceAbsolute verify -database $restoredDatabase
    if ($LASTEXITCODE -ne 0) { throw "Restored database validation failed." }

    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $databaseSafetyCopy = "$databaseAbsolute.pre-restore-$timestamp"
    $uploadsSafetyCopy = "$uploadsAbsolute.pre-restore-$timestamp"
    New-Item -ItemType Directory -Path (Split-Path -Parent $databaseAbsolute) -Force | Out-Null
    if (Test-Path -LiteralPath $databaseAbsolute) {
        Move-Item -LiteralPath $databaseAbsolute -Destination $databaseSafetyCopy
    }
    Copy-Item -LiteralPath $restoredDatabase -Destination $databaseAbsolute
    if (Test-Path -LiteralPath $uploadsAbsolute) {
        Move-Item -LiteralPath $uploadsAbsolute -Destination $uploadsSafetyCopy
    }
    $restoredUploads = Join-Path $temporary "uploads"
    if (Test-Path -LiteralPath $restoredUploads -PathType Container) {
        Copy-Item -LiteralPath $restoredUploads -Destination $uploadsAbsolute -Recurse
    } else {
        New-Item -ItemType Directory -Path $uploadsAbsolute -Force | Out-Null
    }
    Write-Host "Restore completed. Previous database: $databaseSafetyCopy"
    if (Test-Path -LiteralPath $uploadsSafetyCopy) { Write-Host "Previous uploads: $uploadsSafetyCopy" }
}
finally {
    if (Test-Path -LiteralPath $temporary) {
        Remove-Item -LiteralPath $temporary -Recurse -Force
    }
}
