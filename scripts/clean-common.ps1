$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$script:CleanRoot = Split-Path $PSScriptRoot -Parent
$script:CleanState = Join-Path $CleanRoot '.runtime\clean'
$script:CleanEnv = Join-Path $CleanRoot '.env.clean.local'
$script:CleanDocker = Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\resources\bin\docker.exe'
$script:CleanGo = Join-Path $env:ProgramFiles 'Go\bin\go.exe'
$script:CleanTunnel = Join-Path ${env:ProgramFiles(x86)} 'cloudflared\cloudflared.exe'
$script:CleanApp = Join-Path $CleanRoot '.runtime\zaiko-clean.exe'
$taskNodeBin = Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin'
$taskPnpmBin = Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\bin\fallback'
$env:PATH = "$taskNodeBin;$taskPnpmBin;$(Split-Path $CleanDocker);$(Split-Path $CleanGo);$env:PATH"

function Protect-CleanPath([string]$Path) {
 $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
 if (Test-Path -LiteralPath $Path -PathType Container) {
  $grant = '*'+$sid.Value+':(OI)(CI)(F)'
 } else { $grant = '*'+$sid.Value+':(F)' }
 & icacls.exe $Path /inheritance:r /grant:r $grant | Out-Null
 if ($LASTEXITCODE -ne 0) { throw 'Cannot protect local credentials.' }
}
function Read-CleanEnv {
 if (-not (Test-Path -LiteralPath $CleanEnv)) { throw 'Run setup-clean.ps1 first.' }
 $values = [ordered]@{}
 foreach ($line in [IO.File]::ReadAllLines($CleanEnv)) {
  if ([string]::IsNullOrWhiteSpace($line) -or $line.StartsWith('#')) { continue }
  $pair = $line.Split('=',2)
  if ($pair.Count -ne 2 -or $pair[0] -notmatch '^ZAIKO_[A-Z0-9_]+$') { throw 'Invalid clean environment file.' }
  $values[$pair[0]] = $pair[1]
 }
 return $values
}
function Write-CleanEnv($Values) {
 if (-not (Test-Path -LiteralPath $CleanEnv)) { [IO.File]::WriteAllText($CleanEnv,'') }
 Protect-CleanPath $CleanEnv
 $lines = foreach ($key in $Values.Keys) { "$key=$($Values[$key])" }
 [IO.File]::WriteAllLines($CleanEnv, [string[]]$lines, [Text.UTF8Encoding]::new($false))
}
function Import-CleanEnv($Values) {
 foreach ($key in @(Get-ChildItem Env: | Where-Object Name -like 'ZAIKO_*')) { Remove-Item -LiteralPath ('Env:'+$key.Name) }
 foreach ($key in $Values.Keys) { [Environment]::SetEnvironmentVariable($key,[string]$Values[$key],'Process') }
}
function Invoke-CleanCompose([string[]]$ComposeArguments) {
 & $CleanDocker compose --env-file $CleanEnv -f (Join-Path $CleanRoot 'compose.clean.yaml') -p zaiko-clean @ComposeArguments
 if ($LASTEXITCODE -ne 0) { throw 'Dedicated Docker Compose operation failed.' }
}
function Test-CleanDockerAppMode {
 if (-not (Test-Path -LiteralPath $CleanEnv)) { return $false }
 $values = Read-CleanEnv
 return $values.Contains('ZAIKO_CLEAN_APP_RUNTIME') -and $values.ZAIKO_CLEAN_APP_RUNTIME -eq 'docker'
}
function Get-CleanDockerApp {
 $ids = & $CleanDocker ps --filter 'name=^zaiko-clean-app-1$' --filter 'label=com.docker.compose.project=zaiko-clean' --filter 'status=running' --format '{{.ID}}' 2>$null
 if ($LASTEXITCODE -eq 0 -and $ids) { return [pscustomobject]@{ Id=[string]$ids; Runtime='docker' } }
 return $null
}
function Start-CleanContainerApp($Values) {
 $linuxValues=[ordered]@{}
 foreach ($key in $Values.Keys) { if ($key -notlike 'ZAIKO_CLEAN_*') { $linuxValues[$key]=$Values[$key] } }
 $linuxValues.ZAIKO_ADDRESS='0.0.0.0:18088'
 $linuxValues.ZAIKO_DATABASE_URL=$Values.ZAIKO_DATABASE_URL.Replace('127.0.0.1:15432','postgres:5432')
 $linuxValues.ZAIKO_DATABASE_PATH='/state/compat.db'
 $linuxValues.ZAIKO_UPLOAD_DIRECTORY='/state/uploads'
 $linuxValues.ZAIKO_PUBLIC_DEMO_PASSWORD_FILE='/state/admin-password.txt'
 $appEnv=Join-Path $CleanRoot '.env.clean.app.local'
 if (-not (Test-Path -LiteralPath $appEnv)) { [IO.File]::WriteAllText($appEnv,'') }
 Protect-CleanPath $appEnv
 $lines=foreach($key in $linuxValues.Keys) { "$key=$($linuxValues[$key])" }
 [IO.File]::WriteAllLines($appEnv,[string[]]$lines,[Text.UTF8Encoding]::new($false))
 & $CleanDocker compose --env-file $CleanEnv -f (Join-Path $CleanRoot 'compose.clean.yaml') -f (Join-Path $CleanRoot 'compose.clean-app.yaml') -p zaiko-clean up -d app
 if ($LASTEXITCODE -ne 0) { throw 'Container app startup failed.' }
 for($attempt=0;$attempt -lt 30;$attempt++) {
  try { $response=Invoke-WebRequest 'http://127.0.0.1:18088/readyz' -TimeoutSec 2; if($response.StatusCode -eq 200) { Write-Output 'Container app ready.'; return } } catch {}
  Start-Sleep -Seconds 1
 }
 throw 'Container app readiness timed out.'
}
function Get-CleanProcess([string]$Kind) {
 if ($Kind -eq 'app' -and (Test-CleanDockerAppMode)) { return Get-CleanDockerApp }

 $file = Join-Path $CleanState "$Kind-process.json"
 if (-not (Test-Path -LiteralPath $file)) { return $null }
 $record = Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
 $process = Get-Process -Id $record.id -ErrorAction SilentlyContinue
 if ($process) {
  if ($process.Path -eq $record.path -and $process.StartTime.ToUniversalTime().Ticks -eq ([datetime]$record.startedAt).ToUniversalTime().Ticks) { return $process }
  throw "Saved $Kind PID does not match the current process; inspect before proceeding."
 }
 return $null
}
function Stop-CleanProcess([string]$Kind) {
 if ($Kind -eq 'app' -and (Test-CleanDockerAppMode)) {
  $container=Get-CleanDockerApp
  if($container) { & $CleanDocker stop --time 10 $container.Id | Out-Null; if($LASTEXITCODE -ne 0) { throw 'Cannot stop container app.' } }
  return
 }

 $process = Get-CleanProcess $Kind
 if ($process) { Stop-Process -Id $process.Id; $process.WaitForExit(10000) | Out-Null }
 $file = Join-Path $CleanState "$Kind-process.json"
 if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file }
}
function Save-CleanProcess([string]$Kind, $Process) {
 # Start-Process can return before Process.Path becomes available on Windows.
 $expectedPath = if ($Kind -eq 'app') { $CleanApp } elseif ($Kind -eq 'tunnel') { $CleanTunnel } else { throw 'Unknown process kind.' }
 @{id=$Process.Id;path=$expectedPath;startedAt=$Process.StartTime.ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $CleanState "$Kind-process.json")
}
function Start-CleanApp($Values) {
 if (Get-CleanProcess 'app') { throw 'This environment app is already running.' }
 if (Test-CleanDockerAppMode) { Start-CleanContainerApp $Values; return }
 Import-CleanEnv $Values
 $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,18088)
 try { $listener.Start() } finally { $listener.Stop() }
 $stamp = Get-Date -Format yyyyMMdd-HHmmss-fff
 $process = Start-Process -FilePath $CleanApp -WorkingDirectory $CleanRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $CleanState "app-$stamp.out.log") -RedirectStandardError (Join-Path $CleanState "app-$stamp.err.log")
 Save-CleanProcess 'app' $process
 for ($attempt=0;$attempt -lt 30;$attempt++) {
  if ($process.HasExited) { throw 'App exited; inspect its private logs.' }
  try { $response = Invoke-WebRequest 'http://127.0.0.1:18088/readyz' -TimeoutSec 2; if ($response.StatusCode -eq 200) { Write-Output "App ready; PID $($process.Id)"; return } } catch {}
  Start-Sleep -Seconds 1
 }
 throw 'App readiness timed out; inspect its private logs.'
}
