$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$localGo = Join-Path $projectRoot '.tooling\go\bin\go.exe'
$goCommand = if (Test-Path -LiteralPath $localGo) { $localGo } else { 'go' }

Set-Location -LiteralPath $projectRoot
& $goCommand run ./cmd/server
