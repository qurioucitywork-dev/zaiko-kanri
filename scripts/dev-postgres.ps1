[CmdletBinding()]
param(
    [int]$Port = 8080,
    [string]$Distro = 'Ubuntu'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$localGo = Join-Path $projectRoot '.tooling\go\bin\go.exe'
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { 'go' }

& (Join-Path $PSScriptRoot 'local-postgres.ps1') -Action start -Distro $Distro

$env:ZAIKO_ADDRESS = "127.0.0.1:$Port"
$env:ZAIKO_PUBLIC_BASE_URL = "http://127.0.0.1:$Port"
$env:ZAIKO_DATABASE_DRIVER = 'postgres'
$env:ZAIKO_DATABASE_URL = 'postgres://zaiko:zaiko-local@127.0.0.1:5432/zaiko?sslmode=disable'
$env:ZAIKO_DATABASE_PATH = Join-Path $projectRoot '.data\zaiko.db'
$env:ZAIKO_STORAGE_DRIVER = 'local'
$env:ZAIKO_UPLOAD_DIRECTORY = Join-Path $projectRoot '.data\uploads'
$env:ZAIKO_ENV = 'development'
$env:ZAIKO_COOKIE_SECURE = 'false'

Set-Location -LiteralPath $projectRoot
Write-Host "在庫管理ツールをPostgreSQLモードで起動します: http://127.0.0.1:$Port/app/"
& $goCommand run ./cmd/server
