param([string]$BaseUrl='http://127.0.0.1:18088',[string]$ReportName='clean-api-verification-local.json')
. "$PSScriptRoot\clean-common.ps1"
$results=[Collections.Generic.List[object]]::new()
foreach($path in @('/healthz','/readyz','/api/v1/health')) {
 $response=Invoke-WebRequest ($BaseUrl+$path) -TimeoutSec 20
 if($response.StatusCode -ne 200){throw "Health failed: $path"}
 if($path -eq '/api/v1/health' -and ($response.Content|ConvertFrom-Json).database.driver -ne 'postgres'){throw 'Wrong DB driver'}
 $results.Add(@{path=$path;status=200})
}
foreach($path in @('/api/v1/products','/api/v1/users','/api/v1/company')) {
 $response=Invoke-WebRequest ($BaseUrl+$path) -SkipHttpErrorCheck -TimeoutSec 20
 if($response.StatusCode -notin 401,403){throw "Unprotected API: $path"}
 $results.Add(@{path=$path;unauthenticatedStatus=[int]$response.StatusCode})
}
$password=[IO.File]::ReadAllText((Join-Path $CleanState 'admin-password.txt')).Trim()
$body=@{username='admin';password=$password}|ConvertTo-Json -Compress
$login=Invoke-WebRequest ($BaseUrl+'/api/v1/auth/login') -Method Post -ContentType 'application/json' -Body $body -SessionVariable cleanSession -TimeoutSec 20
$password=$null;$body=$null
$me=$login.Content|ConvertFrom-Json
if($me.user.role -ne 'admin'){throw 'Administrator login failed'}
$cookies=@($cleanSession.Cookies.GetCookies([uri]$BaseUrl))
if($cookies.Count -lt 2 -or @($cookies|Where-Object {-not $_.HttpOnly}).Count){throw 'HttpOnly cookie check failed'}
if($BaseUrl.StartsWith('https://') -and @($cookies|Where-Object {-not $_.Secure}).Count){throw 'Secure cookie check failed'}
$results.Add(@{path='/api/v1/auth/login';status=200;role='admin';httpOnly=$true;secure=$BaseUrl.StartsWith('https://')})
$paths=@('/products?includeCancelled=true','/purchases','/sales','/parts','/cost-adjustments','/partners?includeInactive=true','/purchase-staff','/boxes','/purchase-requests','/notifications','/approvals','/shipments','/consignments','/returns','/exchange-rates','/market-prices')
$paths+=@('brands','materials','movements','conditions','accessories','auctions','belt-materials','dials','product-shapes','markings','part-names')|ForEach-Object {'/masters/'+$_}
foreach($path in $paths) {
 $response=Invoke-WebRequest ($BaseUrl+'/api/v1'+$path) -WebSession $cleanSession -TimeoutSec 20 -SkipHttpErrorCheck
 if($response.StatusCode -ne 200){throw "API check failed: $path status $($response.StatusCode)"}
 $payload=$response.Content|ConvertFrom-Json
 if(-not ($payload.PSObject.Properties.Name -contains 'items')){throw "Missing items: $path"}
 $count=if($null -eq $payload.items){0}else{@($payload.items).Count}
 if($count -ne 0){throw "Unexpected business data: $path ($count)"}
 $results.Add(@{path='/api/v1'+$path;status=200;count=0})
}
$company=Invoke-RestMethod ($BaseUrl+'/api/v1/company') -WebSession $cleanSession -TimeoutSec 20
foreach($field in @('postalCode','address','phone','fax','email','invoiceNumber','representativeName')) {if($company.$field){throw "Company field is not blank: $field"}}
if($company.bankAccounts){throw 'Company bank accounts are not empty'}
$results.Add(@{path='/api/v1/company';status=200;blankProfile=$true})
$invalid=Invoke-WebRequest ($BaseUrl+'/api/v1/auth/logout') -Method Post -WebSession $cleanSession -SkipHttpErrorCheck -TimeoutSec 20
if($invalid.StatusCode -ne 403){throw 'Missing CSRF token was not rejected'}
$logout=Invoke-WebRequest ($BaseUrl+'/api/v1/auth/logout') -Method Post -WebSession $cleanSession -Headers @{'X-CSRF-Token'=$me.csrfToken} -TimeoutSec 20
if($logout.StatusCode -ne 204){throw 'CSRF authenticated logout failed'}
$after=Invoke-WebRequest ($BaseUrl+'/api/v1/products') -WebSession $cleanSession -SkipHttpErrorCheck -TimeoutSec 20
if($after.StatusCode -ne 401){throw 'Logged-out session still has access'}
$results.Add(@{path='/api/v1/auth/logout';status=204;missingCsrfStatus=403;afterLogoutStatus=401})
@{baseUrl=$BaseUrl;checkedAt=(Get-Date).ToString('o');checks=$results}|ConvertTo-Json -Depth 6|Set-Content -LiteralPath (Join-Path $CleanRoot "docs\$ReportName")
Write-Output "Verified $($results.Count) health/auth/empty API checks at $BaseUrl"
