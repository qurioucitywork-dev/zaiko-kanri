param([switch]$SkipFrontend)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
Push-Location $repoRoot
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    if (-not $SkipFrontend) {
        pnpm --dir frontend install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency install failed' }
        pnpm --dir frontend run build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    }
    New-Item -ItemType Directory -Force (Join-Path $PSScriptRoot 'build') | Out-Null
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags='-s -w' -o (Join-Path $PSScriptRoot 'build/zaiko-server') ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Go cross compilation failed' }
    # Native DO scheduling keeps a running process until the application asks
    # it to restart. A successful build therefore advances the release marker.
    $releaseConfigPath = Join-Path $PSScriptRoot 'wrangler.jsonc'
    $releaseConfig = Get-Content $releaseConfigPath -Raw | ConvertFrom-Json
    $releaseConfig.vars.APP_REVISION = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $releaseConfig | ConvertTo-Json -Depth 12 | Set-Content $releaseConfigPath -Encoding utf8
} finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}

