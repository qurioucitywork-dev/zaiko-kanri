param([switch]$StopDatabase)
. "$PSScriptRoot\clean-common.ps1"
Stop-CleanProcess 'tunnel'
Stop-CleanProcess 'app'
if ($StopDatabase) { Invoke-CleanCompose @('stop','postgres') }
Write-Output 'Dedicated app/tunnel stopped. Database volume and uploads preserved.'
