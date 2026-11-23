. "$PSScriptRoot\clean-common.ps1"
if (Get-CleanProcess 'tunnel') {
 if ((Get-CleanProcess 'app') -and (Read-CleanEnv).ZAIKO_ENV -eq 'production') {
  $currentUrl=(Read-CleanEnv).ZAIKO_PUBLIC_BASE_URL
  try {
   $publicReady=Invoke-WebRequest ($currentUrl+'/readyz') -TimeoutSec 12 -SkipHttpErrorCheck
   if ($publicReady.StatusCode -eq 200) { Write-Output ($currentUrl+'/app/admin-reference/index.html'); return }
  } catch {}
  Write-Output 'Existing Quick Tunnel is unreachable; issuing a replacement URL.'
 }
 # Recover an interrupted hostname/startup phase using only our recorded process.
 Stop-CleanProcess 'tunnel'
}
if (-not (Get-CleanProcess 'app')) { & "$PSScriptRoot\start-clean.ps1" }
# Refuse publishing if unauthenticated clients can read protected data.
foreach ($path in @('/api/v1/products','/api/v1/users','/api/v1/company')) {
 $response=Invoke-WebRequest ("http://127.0.0.1:18088"+$path) -SkipHttpErrorCheck -TimeoutSec 10
 if ($response.StatusCode -notin 401,403) { throw "Protected API check failed: $path" }
}
Write-Output 'Publishing this PC inventory management app through HTTPS; application login remains required.'
$stamp=Get-Date -Format yyyyMMdd-HHmmss-fff
$out=Join-Path $CleanState "tunnel-$stamp.out.log"
$err=Join-Path $CleanState "tunnel-$stamp.err.log"
$config=Join-Path $CleanState 'tunnel-config.yaml'
[IO.File]::WriteAllText($config,"no-autoupdate: true`n")
# Stop origin while the public hostname is issued, avoiding an insecure-cookie interval.
Stop-CleanProcess 'app'
$process=Start-Process -FilePath $CleanTunnel -ArgumentList @('tunnel','--config', $config,'--url','http://127.0.0.1:18088') -WorkingDirectory $CleanRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
Save-CleanProcess 'tunnel' $process
$publicUrl=$null
for ($attempt=0;$attempt -lt 45;$attempt++) {
 if ($process.HasExited) { throw 'Quick Tunnel exited; inspect private logs.' }
 $logs=''
 foreach ($file in @($err,$out)) {
  if (-not (Test-Path -LiteralPath $file)) { continue }
  $stream=[IO.File]::Open($file,[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::ReadWrite)
  $reader=[IO.StreamReader]::new($stream)
  try { $logs += $reader.ReadToEnd() } finally { $reader.Dispose();$stream.Dispose() }
 }
 $match=[regex]::Match($logs,'https://[a-z0-9-]+\.trycloudflare\.com')
 if ($match.Success) { $publicUrl=$match.Value;break }
 Start-Sleep -Seconds 1
}
if (-not $publicUrl) { Stop-CleanProcess 'tunnel';throw 'Quick Tunnel URL was not issued.' }
$values=Read-CleanEnv
$values.ZAIKO_ENV='production';$values.ZAIKO_COOKIE_SECURE='true';$values.ZAIKO_PUBLIC_BASE_URL=$publicUrl
Write-CleanEnv $values
try { Start-CleanApp $values } catch { Stop-CleanProcess 'tunnel';throw }
$publicUrl | Set-Content -LiteralPath (Join-Path $CleanState 'public-url.txt')
Write-Output ($publicUrl+'/app/admin-reference/index.html')
