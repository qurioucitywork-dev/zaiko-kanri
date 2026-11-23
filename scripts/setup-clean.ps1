param([switch]$PrepareOnly)
. "$PSScriptRoot\clean-common.ps1"
Push-Location $CleanRoot
try {
 & "$PSScriptRoot\check-clean-prerequisites.ps1" -RepositoryPath $CleanRoot
 if ($LASTEXITCODE -ne 0) { throw 'Prerequisites failed.' }
 if (-not (Test-Path -LiteralPath $CleanEnv)) {
  $existingVolumes = & $CleanDocker volume ls --format '{{.Name}}'
  $existingContainers = & $CleanDocker ps -a --filter label=com.docker.compose.project=zaiko-clean --format '{{.Names}}'
  if ($LASTEXITCODE -ne 0 -or $existingVolumes -contains 'zaiko-clean_postgres-clean-data' -or $existingContainers) { throw 'Dedicated name already exists. Inspect ownership before setup.' }
  $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,15432)
  try { $listener.Start() } finally { $listener.Stop() }
  New-Item -ItemType Directory -Path $CleanState -Force | Out-Null
  Protect-CleanPath $CleanState
  New-Item -ItemType Directory -Path (Join-Path $CleanState 'uploads') -Force | Out-Null
  $dbSecret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
  $adminSecret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
  [IO.File]::WriteAllText((Join-Path $CleanState 'admin-password.txt'),$adminSecret)
  Write-CleanEnv ([ordered]@{
   ZAIKO_CLEAN_DB_PASSWORD=$dbSecret
   ZAIKO_ENV='test';ZAIKO_ADDRESS='127.0.0.1:18088';ZAIKO_DATABASE_DRIVER='postgres'
   ZAIKO_DATABASE_URL="postgres://zaiko_clean:${dbSecret}@127.0.0.1:15432/zaiko_clean?sslmode=disable"
   ZAIKO_DATABASE_PATH=(Join-Path $CleanState 'compat.db');ZAIKO_UPLOAD_DIRECTORY=(Join-Path $CleanState 'uploads')
   ZAIKO_STORAGE_DRIVER='local';ZAIKO_ORGANIZATION_CODE='CLEAN';ZAIKO_COOKIE_SECURE='false';ZAIKO_PUBLIC_BASE_URL='http://127.0.0.1:18088'
  })
 }
 Invoke-CleanCompose @('up','-d','--wait','postgres')
 if ($PrepareOnly) { Write-Output 'Dedicated database ready; not initialized.'; return }
 if (Get-CleanProcess 'app') { throw 'Stop this app before rebuilding; no DB data is removed.' }
 Push-Location frontend
 try {
  pnpm dlx pnpm@11.9.0 install --frozen-lockfile
  if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed.' }
  pnpm dlx pnpm@11.9.0 run build
  if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
  node tests/clean-state.test.mjs
  if ($LASTEXITCODE -ne 0) { throw 'Connected empty-state tests failed.' }
  pnpm dlx pnpm@11.9.0 run test:reference
  if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed.' }
 } finally { Pop-Location }
 foreach ($entry in @(Get-ChildItem Env: | Where-Object Name -like 'ZAIKO_*')) { Remove-Item -LiteralPath ('Env:'+$entry.Name) }
 $env:ZAIKO_ENV='test'
 & $CleanGo test ./...
 if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
 & $CleanGo build -trimpath -o $CleanApp ./cmd/server
 if ($LASTEXITCODE -ne 0) { throw 'App build failed.' }
 $bootstrap = Join-Path $CleanRoot '.runtime\bootstrap-clean.exe'
 & $CleanGo build -trimpath -o $bootstrap ./cmd/bootstrap-clean
 if ($LASTEXITCODE -ne 0) { throw 'Bootstrap build failed.' }
 Import-CleanEnv (Read-CleanEnv)
 & $bootstrap --password-file (Join-Path $CleanState 'admin-password.txt') --username admin
 if ($LASTEXITCODE -eq 3) { Write-Output 'Existing environment preserved. No credentials or business data were overwritten.' }
 elseif ($LASTEXITCODE -ne 0) { throw 'Clean bootstrap failed.' }
 Write-Output 'Build/setup complete. Run start-clean.ps1.'
} finally { Pop-Location }
