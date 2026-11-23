param([string]$RepositoryPath = 'C:\Work\zaiko-kanri')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Read-only checks. Never starts an application, initializes a DB, or accepts a license.
$root = (Resolve-Path -LiteralPath $RepositoryPath).Path
function Read-Source([string]$RelativePath) {
    $path = Join-Path $root $RelativePath
    if (Test-Path -LiteralPath $path -PathType Leaf) {
        return [IO.File]::ReadAllText($path)
    }
    return ''
}

Push-Location $root
try {
    Write-Output ('Repository: ' + $root)
    git remote get-url origin
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read repository origin.' }
    git branch --show-current
    git rev-parse HEAD
    git status --short

    $sales = Read-Source 'internal/persistence/sales_adjustments.go'
    $app = Read-Source 'frontend/public/admin-reference/js/app.js'
    $html = Read-Source 'frontend/public/admin-reference/app.html'
    $checks = [ordered]@{
        'sales_adjustments.go with adjustmentDate' = ($sales.Contains('adjustmentDate'))
        'migration 000083' = (Test-Path -LiteralPath (Join-Path $root 'internal/persistence/migrations/000083_sales_adjustment_line_directions.up.sql'))
        'sales change modal' = ($app.Contains('openSalesChangeModal'))
        'gross margin sort function' = ($app.Contains('toggleInventoryGrossMarginSort'))
        'gross margin sort button' = ($html.Contains('inv-gross-margin-sort-btn'))
    }
    $missing = @()
    foreach ($entry in $checks.GetEnumerator()) {
        Write-Output ('{0}: {1}' -f $entry.Key, $entry.Value)
        if (-not $entry.Value) { $missing += $entry.Key }
    }

    $go = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
    $docker = Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\resources\bin\docker.exe'
    $cloudflared = Join-Path ${env:ProgramFiles(x86)} 'cloudflared\cloudflared.exe'
    foreach ($exe in @($go, $docker, $cloudflared)) {
        if (-not (Test-Path -LiteralPath $exe)) { throw "Tool not found: $exe" }
    }
    & $go version
    if ($LASTEXITCODE -ne 0) { throw 'Go is unavailable.' }
    node --version
    if ($LASTEXITCODE -ne 0) { throw 'Node is unavailable in this shell.' }
    & $docker compose version
    if ($LASTEXITCODE -ne 0) { throw 'Docker Compose is unavailable.' }
    & $docker info --format '{{.ServerVersion}}'
    if ($LASTEXITCODE -ne 0) { throw 'Open Docker Desktop and complete its initial setup.' }
    & $cloudflared --version
    if ($LASTEXITCODE -ne 0) { throw 'cloudflared is unavailable.' }

    if ($missing.Count -gt 0) {
        Write-Output 'BLOCKED: Required source changes have not been shared. Do not initialize or publish this version.'
        exit 2
    }
    Write-Output 'Source markers found. Review implementation, date capture, tax display, migrations, and empty-data behavior before setup.'
    Write-Output 'This check does not initialize or start the application.'
} finally {
    Pop-Location
}
