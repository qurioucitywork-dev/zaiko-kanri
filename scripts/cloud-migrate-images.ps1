#requires -Version 7.2
<#
.SYNOPSIS
Copies manifest-listed local images/PDFs to the private Cloudflare R2 gateway.
.DESCRIPTION
Default mode validates every local file without making a network request.
-Apply checks each remote key, skips identical data, refuses conflicts, uploads
missing objects, and downloads every object to verify its SHA-256. No database,
source file, or remote object is deleted. Existing objects are never intentionally
overwritten. Run while uploads are paused to avoid a concurrent key-creation race.
Manifest is a JSON array of object_key, local_path, content_type, size_bytes, sha256.
SHA-256 must describe the exact source bytes; generate it from the local files.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ManifestPath,
    [Parameter(Mandatory)][string]$UploadRoot,
    [Parameter(Mandatory)][string]$OrganizationId,
    [Parameter(Mandatory)][ValidateRange(1, 100000)][int]$ExpectedCount,
    [Parameter(Mandatory)][string]$GatewayUrl,
    [string]$SecretsFile,
    [string]$ReportPath,
    [switch]$Apply
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-ByteHash([byte[]]$Bytes) {
    return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($Bytes)).ToLowerInvariant()
}
function Assert-NoReparsePoint([string]$Path, [string]$Root) {
    $current = Get-Item -LiteralPath $Path -Force
    while ($null -ne $current) {
        if (($current.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Source path contains a symbolic link or reparse point; resolve it before migration.'
        }
        if ($current.FullName.TrimEnd('\', '/') -eq $Root.TrimEnd('\', '/')) { return }
        $current = if ($current -is [IO.DirectoryInfo]) { $current.Parent } else { $current.Directory }
    }
    throw 'Could not verify the source path is within UploadRoot.'
}
function Write-Report {
    if (-not $script:report) { return }
    $script:report.finished_at_utc = [DateTime]::UtcNow.ToString('o')
    $script:report | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $script:resolvedReportPath -Encoding utf8
}

$gateway = [Uri]$GatewayUrl
if (-not $gateway.IsAbsoluteUri -or $gateway.Scheme -ne 'https' -or $gateway.UserInfo -or $gateway.Query -or $gateway.Fragment -or $gateway.AbsolutePath.TrimEnd('/') -ne '/__storage') {
    throw 'GatewayUrl must be an HTTPS URL ending in /__storage without credentials, query, or fragment.'
}
if ($OrganizationId -notmatch '^org_[A-Za-z0-9_-]+$') { throw 'Invalid organization ID.' }
$resolvedRoot = (Resolve-Path -LiteralPath $UploadRoot).ProviderPath.TrimEnd('\', '/')
if (-not (Get-Item -LiteralPath $resolvedRoot).PSIsContainer) { throw 'UploadRoot must be a directory.' }
$sourcePrefix = $resolvedRoot + [IO.Path]::DirectorySeparatorChar
$resolvedManifest = (Resolve-Path -LiteralPath $ManifestPath).ProviderPath
$manifest = @(Get-Content -LiteralPath $resolvedManifest -Raw | ConvertFrom-Json -Depth 20)
if ($manifest.Count -ne $ExpectedCount) { throw "Manifest count mismatch: expected $ExpectedCount, found $($manifest.Count)." }
if (-not $ReportPath) { $ReportPath = Join-Path (Split-Path $resolvedManifest -Parent) ('r2-migration-report-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '.json') }
$script:resolvedReportPath = [IO.Path]::GetFullPath($ReportPath)
if ($script:resolvedReportPath -eq $resolvedManifest -or $script:resolvedReportPath.StartsWith($sourcePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'ReportPath must not overwrite the manifest or a source upload.'
}
if (-not (Test-Path -LiteralPath (Split-Path $script:resolvedReportPath -Parent) -PathType Container)) { throw 'Report output directory does not exist.' }

$validated = [Collections.Generic.List[object]]::new()
$keys = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
[long]$totalBytes = 0
foreach ($entry in $manifest) {
    foreach ($field in @('object_key','local_path','content_type','size_bytes','sha256')) {
        if ($entry.PSObject.Properties.Name -notcontains $field) { throw "Manifest entry missing field: $field" }
    }
    $key = [string]$entry.object_key
    if (-not $keys.Add($key)) { throw 'Duplicate object key in manifest.' }
    if ($key.Length -gt 1024 -or $key -match '[\\\x00-\x1f\x7f]' -or @($key.Split('/') | Where-Object { $_ -in @('', '.', '..') }).Count -gt 0) { throw 'Unsafe object key in manifest.' }
    if (-not ($key.StartsWith("organizations/$OrganizationId/", [StringComparison]::Ordinal) -or $key.StartsWith("official-documents/$OrganizationId/", [StringComparison]::Ordinal))) {
        throw 'Object key belongs to a different organization or unsupported namespace.'
    }
    $local = [string]$entry.local_path
    if (-not [IO.Path]::IsPathRooted($local)) { $local = Join-Path $resolvedRoot $local }
    $resolvedFile = (Resolve-Path -LiteralPath $local).ProviderPath
    if (-not $resolvedFile.StartsWith($sourcePrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Local file escapes UploadRoot.' }
    Assert-NoReparsePoint $resolvedFile $resolvedRoot
    $file = Get-Item -LiteralPath $resolvedFile
    if ($file.PSIsContainer -or $file.Length -le 0 -or $file.Length -gt 25MB) { throw 'Source file must contain 1 to 25 MiB.' }
    if ([long]$entry.size_bytes -ne $file.Length) { throw 'Source file length differs from manifest.' }
    if ([string]$entry.sha256 -notmatch '^[0-9a-fA-F]{64}$') { throw 'Manifest SHA-256 must be 64 hexadecimal characters.' }
    $hash = (Get-FileHash -LiteralPath $resolvedFile -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne ([string]$entry.sha256).ToLowerInvariant()) { throw 'Source file SHA-256 differs from manifest.' }
    $contentType = [string]$entry.content_type
    if ($contentType -notin @('image/jpeg','image/png','image/webp','application/pdf','application/octet-stream')) { throw 'Unsupported manifest content type.' }
    $encodedKey = ($key.Split('/') | ForEach-Object { [Uri]::EscapeDataString($_) }) -join '/'
    $validated.Add([pscustomobject]@{ object_key=$key; local_path=$resolvedFile; size_bytes=[long]$file.Length; sha256=$hash; content_type=$contentType; url=$GatewayUrl.TrimEnd('/')+'/'+$encodedKey })
    $totalBytes += $file.Length
}
$script:report = [ordered]@{
    mode = $(if ($Apply) { 'apply' } else { 'dry-run' })
    organization_id = $OrganizationId
    gateway_host = $gateway.Host
    started_at_utc = [DateTime]::UtcNow.ToString('o')
    finished_at_utc = ''
    manifest_count = $validated.Count
    total_bytes = $totalBytes
    uploaded = 0
    already_matching = 0
    verified = 0
    succeeded = $false
    files = [Collections.Generic.List[object]]::new()
}
if (-not $Apply) {
    foreach ($item in $validated) { $script:report.files.Add([pscustomobject]@{object_key=$item.object_key; size_bytes=$item.size_bytes; sha256=$item.sha256; status='local-validated'}) }
    $script:report.succeeded = $true
    Write-Report
    Write-Output "Dry run passed: $($validated.Count) files, $totalBytes bytes. No network requests or uploads performed."
    Write-Output "Report: $script:resolvedReportPath"
    return
}
if (-not $SecretsFile) { throw 'SecretsFile is required with -Apply.' }
$secrets = Get-Content -LiteralPath $SecretsFile -Raw | ConvertFrom-Json
$token = [string]$secrets.R2_GATEWAY_TOKEN
if ($token.Length -lt 32) { throw 'Missing or invalid R2 gateway token.' }
$handler = [Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(90)
$client.DefaultRequestHeaders.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)
try {
    foreach ($item in $validated) {
        # Verify unchanged source bytes again immediately before any remote mutation.
        $bytes = [IO.File]::ReadAllBytes($item.local_path)
        if ($bytes.LongLength -ne $item.size_bytes -or (Get-ByteHash $bytes) -ne $item.sha256) { throw 'Source file changed after preflight.' }
        $existing = $client.GetAsync($item.url).GetAwaiter().GetResult()
        try {
            if ([int]$existing.StatusCode -eq 200) {
                $remoteBytes = $existing.Content.ReadAsByteArrayAsync().GetAwaiter().GetResult()
                if ($remoteBytes.LongLength -ne $item.size_bytes -or (Get-ByteHash $remoteBytes) -ne $item.sha256) { throw 'Remote object already exists with different bytes; refused to overwrite.' }
                $script:report.already_matching++
                $script:report.verified++
                $script:report.files.Add([pscustomobject]@{object_key=$item.object_key; size_bytes=$item.size_bytes; sha256=$item.sha256; status='already-matching'})
                Write-Report
                continue
            }
            if ([int]$existing.StatusCode -ne 404) { throw "Remote read failed with HTTP $([int]$existing.StatusCode); no upload attempted." }
        } finally { $existing.Dispose() }
        $content = [Net.Http.ByteArrayContent]::new($bytes)
        $content.Headers.ContentType = [Net.Http.Headers.MediaTypeHeaderValue]::new($item.content_type)
        try {
            $uploaded = $client.PutAsync($item.url, $content).GetAwaiter().GetResult()
            try {
                if ([int]$uploaded.StatusCode -ne 200) { throw "Upload failed with HTTP $([int]$uploaded.StatusCode)." }
                $saved = $uploaded.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
                if ([long]$saved.size -ne $item.size_bytes) { throw 'Remote reported upload size mismatch.' }
            } finally { $uploaded.Dispose() }
        } finally { $content.Dispose() }
        $script:report.uploaded++
        $download = $client.GetAsync($item.url).GetAwaiter().GetResult()
        try {
            if ([int]$download.StatusCode -ne 200) { throw "Verification read failed with HTTP $([int]$download.StatusCode)." }
            $remoteBytes = $download.Content.ReadAsByteArrayAsync().GetAwaiter().GetResult()
            if ($remoteBytes.LongLength -ne $item.size_bytes -or (Get-ByteHash $remoteBytes) -ne $item.sha256) { throw 'Uploaded object failed byte-for-byte verification.' }
            if ($download.Content.Headers.ContentType.MediaType -ne $item.content_type) { throw 'Uploaded object content type differs from manifest.' }
        } finally { $download.Dispose() }
        $script:report.verified++
        $script:report.files.Add([pscustomobject]@{object_key=$item.object_key; size_bytes=$item.size_bytes; sha256=$item.sha256; status='uploaded-and-verified'})
        Write-Report
    }
    $script:report.succeeded = $script:report.verified -eq $ExpectedCount
    Write-Report
    Write-Output "Completed: $($script:report.verified)/$ExpectedCount verified, $($script:report.uploaded) uploaded, $($script:report.already_matching) already matching."
    Write-Output "Report: $script:resolvedReportPath"
} finally {
    Write-Report
    $client.Dispose()
    $handler.Dispose()
    $token = $null
    $secrets = $null
}
