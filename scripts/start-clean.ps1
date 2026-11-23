. "$PSScriptRoot\clean-common.ps1"
Invoke-CleanCompose @('up','-d','--wait','postgres')
if (Get-CleanProcess 'app') { Write-Output 'This environment is already running.'; return }
$values = Read-CleanEnv
if (-not (Get-CleanProcess 'tunnel')) {
 $values.ZAIKO_ENV='test'; $values.ZAIKO_COOKIE_SECURE='false'; $values.ZAIKO_PUBLIC_BASE_URL='http://127.0.0.1:18088'
 Write-CleanEnv $values
}
Start-CleanApp $values
Write-Output ($values.ZAIKO_PUBLIC_BASE_URL + '/app/admin-reference/index.html')
