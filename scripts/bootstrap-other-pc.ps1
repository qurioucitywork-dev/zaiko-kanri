[CmdletBinding()]
param(
    [switch]$InstallMissing,
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot

$requirements = @(
    @{ Name = 'Git'; Command = 'git'; WingetId = 'Git.Git'; Required = $true },
    @{ Name = 'Docker Desktop'; Command = 'docker'; WingetId = 'Docker.DockerDesktop'; Required = $true },
    @{ Name = 'Go 1.26+'; Command = 'go'; WingetId = 'GoLang.Go'; Required = $true },
    @{ Name = 'Node.js LTS'; Command = 'node'; WingetId = 'OpenJS.NodeJS.LTS'; Required = $true },
    @{ Name = 'GitHub CLI'; Command = 'gh'; WingetId = 'GitHub.cli'; Required = $false }
)

$missing = @()
foreach ($item in $requirements) {
    if (Get-Command $item.Command -ErrorAction SilentlyContinue) {
        Write-Host "[OK] $($item.Name)"
    }
    else {
        Write-Warning "[未導入] $($item.Name)"
        $missing += $item
    }
}

if ($InstallMissing -and $missing.Count -gt 0) {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw 'wingetがありません。docs/other-pc-handoff.mdの公式URLから手動でインストールしてください。'
    }
    foreach ($item in $missing) {
        Write-Host "インストール: $($item.Name)"
        & winget install --id $item.WingetId --exact --accept-package-agreements --accept-source-agreements
        if ($LASTEXITCODE -ne 0) { throw "$($item.Name)のインストールに失敗しました。" }
    }
    Write-Host 'インストール後、Docker Desktopを起動し、必要ならWindowsを再起動してから本スクリプトを再実行してください。'
    exit 0
}

$requiredMissing = @($missing | Where-Object { $_.Required })
if ($requiredMissing.Count -gt 0) {
    throw '必須アプリが不足しています。-InstallMissingを付けるか、引き継ぎ手順に従ってインストールしてください。'
}

& docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Docker Desktopが起動していません。起動後に再実行してください。' }

Write-Host 'PostgreSQLを起動します。'
& docker compose up -d postgres
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQLコンテナの起動に失敗しました。' }

Write-Host 'Go依存関係を取得します。'
& go mod download
if ($LASTEXITCODE -ne 0) { throw 'go mod downloadに失敗しました。' }

Write-Host 'React依存関係を取得してビルドします。'
Push-Location -LiteralPath (Join-Path $projectRoot 'frontend')
try {
    & npm ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ciに失敗しました。' }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Reactビルドに失敗しました。' }
    if (-not $SkipTests) {
        & npm run test:reference
        if ($LASTEXITCODE -ne 0) { throw 'フロントエンドテストに失敗しました。' }
    }
}
finally {
    Pop-Location
}

if (-not $SkipTests) {
    Write-Host 'Goテストを実行します。'
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Goテストに失敗しました。' }
}

Write-Host ''
Write-Host '別PCの開発環境を準備できました。次のコマンドで起動してください。'
Write-Host '  .\scripts\dev-docker-postgres.ps1 -Port 18086'
Write-Host '起動後: http://127.0.0.1:18086/app/app.html'
