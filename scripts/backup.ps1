param(
    [string]$DatabasePath = ".data\zaiko.db",
    [string]$UploadDirectory = ".data\uploads",
    [string]$BackupDirectory = ".backups",
    [string]$MaintenanceBinary = "bin\zaiko-maintenance.exe"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$databaseAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $DatabasePath))
$uploadsAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $UploadDirectory))
$backupRoot = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $BackupDirectory))
$maintenanceAbsolute = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $MaintenanceBinary))
if (-not (Test-Path -LiteralPath $databaseAbsolute -PathType Leaf)) {
    throw "Database not found: $databaseAbsolute"
}
if (-not (Test-Path -LiteralPath $maintenanceAbsolute -PathType Leaf)) {
    throw "Maintenance tool not found. Run scripts\release-check.ps1 first."
}
New-Item -ItemType Directory -Path $backupRoot -Force | Out-Null
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$output = Join-Path $backupRoot "zaiko-backup-$timestamp.zip"
& $maintenanceAbsolute backup -database $databaseAbsolute -uploads $uploadsAbsolute -output $output
if ($LASTEXITCODE -ne 0) { throw "Backup failed." }
Write-Host "Backup completed: $output"
