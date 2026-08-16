[CmdletBinding()]
param(
    [int]$Port = 18086
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$localGo = Join-Path $projectRoot '.tooling\go\bin\go.exe'
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { 'go' }

Set-Location -LiteralPath $projectRoot

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker Desktopが見つかりません。Docker Desktopをインストールして起動してください。'
}
if (-not (Get-Command $goCommand -ErrorAction SilentlyContinue)) {
    throw 'Goが見つかりません。Go 1.26以降をインストールしてください。'
}

& docker compose up -d postgres
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQLコンテナを起動できませんでした。' }

$ready = $false
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    & docker compose exec -T postgres pg_isready -U zaiko -d zaiko *> $null
    if ($LASTEXITCODE -eq 0) {
        $ready = $true
        break
    }
    Start-Sleep -Seconds 1
}
if (-not $ready) { throw 'PostgreSQLが30秒以内に準備完了しませんでした。' }

$env:ZAIKO_ADDRESS = "127.0.0.1:$Port"
$env:ZAIKO_PUBLIC_BASE_URL = "http://127.0.0.1:$Port"
$env:ZAIKO_DATABASE_DRIVER = 'postgres'
$env:ZAIKO_DATABASE_URL = 'postgres://zaiko:zaiko-local@127.0.0.1:5432/zaiko?sslmode=disable'
$env:ZAIKO_DATABASE_PATH = Join-Path $projectRoot '.data\zaiko.db'
$env:ZAIKO_STORAGE_DRIVER = 'local'
$env:ZAIKO_UPLOAD_DIRECTORY = Join-Path $projectRoot '.data\uploads'
$env:ZAIKO_ENV = 'development'
$env:ZAIKO_COOKIE_SECURE = 'false'

Write-Host "在庫管理ツールをDocker PostgreSQLモードで起動します: http://127.0.0.1:$Port/app/app.html"
& $goCommand run ./cmd/server
