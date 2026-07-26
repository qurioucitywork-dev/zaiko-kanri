param(
    [string]$OutputDirectory = "bin"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$localGo = Join-Path $projectRoot ".tooling\go\bin\go.exe"
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { "go" }
$outputRoot = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $OutputDirectory))
$expectedOutputRoot = [System.IO.Path]::GetFullPath((Join-Path $projectRoot "bin"))
if ($outputRoot -ne $expectedOutputRoot) {
    throw "Output is restricted to the project bin directory."
}

Set-Location -LiteralPath $projectRoot
& $goCommand test ./...
if ($LASTEXITCODE -ne 0) { throw "go test failed." }
& $goCommand vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet failed." }
New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
& $goCommand build -trimpath -o (Join-Path $outputRoot "zaiko-kanri.exe") ./cmd/server
if ($LASTEXITCODE -ne 0) { throw "server build failed." }
& $goCommand build -trimpath -o (Join-Path $outputRoot "zaiko-maintenance.exe") ./cmd/maintenance
if ($LASTEXITCODE -ne 0) { throw "maintenance build failed." }
Write-Host "Release checks completed: $outputRoot"
