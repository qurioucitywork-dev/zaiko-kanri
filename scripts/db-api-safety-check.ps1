param(
    [switch]$RunGoTests
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$migrationRoot = Join-Path $projectRoot "internal\database\migrations"
$manifestPath = Join-Path $projectRoot "docs\db-api\legacy-migrations.sha256"
$routerConfigPath = Join-Path $projectRoot "deploy\cloudflare\container-router\wrangler.example.jsonc"
$d1ConfigPath = Join-Path $projectRoot "deploy\cloudflare\d1-service\wrangler.example.jsonc"

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

Assert-True (Test-Path -LiteralPath $manifestPath) "Legacy migration manifest is missing."

$manifestEntries = @{}
foreach ($line in Get-Content -LiteralPath $manifestPath -Encoding UTF8) {
    if ($line -match '^(?<hash>[0-9a-f]{64})\s{2}(?<name>.+\.up\.sql)$') {
        $manifestEntries[$Matches.name] = $Matches.hash
    }
}
Assert-True ($manifestEntries.Count -eq 27) "Expected 27 immutable legacy up migrations."

foreach ($entry in $manifestEntries.GetEnumerator()) {
    $migrationPath = Join-Path $migrationRoot $entry.Key
    Assert-True (Test-Path -LiteralPath $migrationPath) "Legacy migration is missing: $($entry.Key)"
    $actualHash = (Get-FileHash -LiteralPath $migrationPath -Algorithm SHA256).Hash.ToLowerInvariant()
    Assert-True ($actualHash -eq $entry.Value) "Legacy migration changed: $($entry.Key)"
}

$routerConfig = Get-Content -Raw -LiteralPath $routerConfigPath -Encoding UTF8
$d1Config = Get-Content -Raw -LiteralPath $d1ConfigPath -Encoding UTF8
$routerConfig | ConvertFrom-Json | Out-Null
$d1Config | ConvertFrom-Json | Out-Null

Assert-True (-not $routerConfig.Contains('"d1_databases"')) "Public Container Router must not have a D1 binding."
Assert-True ($routerConfig.Contains('"binding": "D1_SERVICE"')) "Container Router service binding is missing."
Assert-True ($d1Config.Contains('"workers_dev": false')) "Internal D1 Worker must disable workers.dev."
Assert-True (-not $d1Config.Contains('"routes"')) "Internal D1 Worker must not declare public routes."
Assert-True ($d1Config.Contains('"d1_databases"')) "Internal D1 Worker D1 binding is missing."
Assert-True ($d1Config.Contains("REPLACE_WITH_TEST_DATABASE_ID")) "Checked-in config must retain placeholder D1 IDs."

$d1WorkerPath = Join-Path $projectRoot "deploy\cloudflare\d1-service\src\index.ts"
$d1Worker = Get-Content -Raw -LiteralPath $d1WorkerPath -Encoding UTF8
foreach ($forbidden in @("request.sql", "request.table", "request.where", "rawSql", "raw_sql")) {
    Assert-True (-not $d1Worker.Contains($forbidden)) "Arbitrary SQL surface detected: $forbidden"
}

if ($RunGoTests) {
    $localGo = Join-Path $projectRoot ".tooling\go\bin\go.exe"
    $goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { "go" }
    $goCache = Join-Path $projectRoot ".gocache"
    $env:GOCACHE = $goCache
    Push-Location -LiteralPath $projectRoot
    try {
        & $goCommand test ./...
        if ($LASTEXITCODE -ne 0) {
            throw "go test failed."
        }
    }
    finally {
        Pop-Location
    }
}

Write-Host "DB/API safety checks passed. No remote resource was changed."
