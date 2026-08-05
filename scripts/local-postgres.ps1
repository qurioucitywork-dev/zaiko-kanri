[CmdletBinding()]
param(
    [ValidateSet('start', 'stop', 'status')]
    [string]$Action = 'status',
    [string]$Distro = 'Ubuntu',
    [int]$Port = 5432
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$stateDirectory = Join-Path $projectRoot '.data'
$statePath = Join-Path $stateDirectory 'postgres-wsl-keeper.json'
$keeperOutputPath = Join-Path $stateDirectory 'postgres-wsl-keeper.out.log'
$keeperErrorPath = Join-Path $stateDirectory 'postgres-wsl-keeper.err.log'
$wslPath = Join-Path $env:SystemRoot 'System32\wsl.exe'

function Get-KeeperProcess {
    if (-not (Test-Path -LiteralPath $statePath)) {
        return $null
    }
    try {
        $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
        if ([int]$state.processId -le 0) {
            return $null
        }
        $process = Get-CimInstance Win32_Process -Filter "ProcessId=$($state.processId)" -ErrorAction Stop
        if ($process.Name -ne 'wsl.exe' -or $process.CommandLine -notlike '*sleep*infinity*') {
            return $null
        }
        return $process
    }
    catch {
        return $null
    }
}

function Find-KeeperProcess {
    try {
        return Get-CimInstance Win32_Process -Filter "Name='wsl.exe'" -ErrorAction Stop |
            Where-Object { $_.CommandLine -like '*sleep*infinity*' } |
            Select-Object -First 1
    }
    catch {
        return $null
    }
}

function Get-KeeperId {
    param($Keeper)

    if ($null -eq $Keeper) {
        return 0
    }
    if ($null -ne $Keeper.PSObject.Properties['Id']) {
        return [int]$Keeper.Id
    }
    return [int]$Keeper.ProcessId
}

function Test-PostgresPort {
    param([int]$TargetPort)

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $task = $client.ConnectAsync('127.0.0.1', $TargetPort)
        if (-not $task.Wait(3000)) {
            return $false
        }
        return $client.Connected
    }
    catch {
        return $false
    }
    finally {
        $client.Dispose()
    }
}

function Show-Status {
    $keeper = Get-KeeperProcess
    $portOpen = Test-PostgresPort -TargetPort $Port
    $keeperStatus = if ($null -ne $keeper) { "running (PID $(Get-KeeperId -Keeper $keeper))" } else { 'stopped' }
    $databaseStatus = if ($portOpen) { 'ready' } else { 'unavailable' }
    Write-Host "WSL keeper: $keeperStatus"
    Write-Host "PostgreSQL localhost:${Port}: $databaseStatus"
    return ($null -ne $keeper -and $portOpen)
}

switch ($Action) {
    'start' {
        New-Item -ItemType Directory -Path $stateDirectory -Force | Out-Null
        $keeper = Get-KeeperProcess
        if ($null -eq $keeper) {
            $keeper = Find-KeeperProcess
        }
        $startedHere = $false
        if ($null -eq $keeper) {
            $arguments = @('-d', $Distro, '-u', 'root', '--', 'sleep', 'infinity')
            $keeper = Start-Process `
                -FilePath $wslPath `
                -ArgumentList $arguments `
                -PassThru `
                -WindowStyle Hidden `
                -RedirectStandardOutput $keeperOutputPath `
                -RedirectStandardError $keeperErrorPath
            $startedHere = $true
            Start-Sleep -Seconds 2
            if ($keeper.HasExited) {
                $detail = Get-Content -LiteralPath $keeperErrorPath -Raw -ErrorAction SilentlyContinue
                throw "WSL keeper failed to start. $detail"
            }
        }

        & $wslPath -d $Distro -u root -- pg_ctlcluster --skip-systemctl-redirect 18 main start 2>$null

        $ready = $false
        for ($attempt = 0; $attempt -lt 15; $attempt++) {
            if (Test-PostgresPort -TargetPort $Port) {
                $ready = $true
                break
            }
            Start-Sleep -Seconds 1
        }
        if (-not $ready) {
            if ($startedHere -and -not $keeper.HasExited) {
                Stop-Process -Id $keeper.Id -Force
            }
            throw "PostgreSQL did not respond on localhost:${Port}."
        }

        @{
            processId = Get-KeeperId -Keeper $keeper
            distro = $Distro
            startedAt = (Get-Date).ToString('o')
        } | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding UTF8

        Write-Host "PostgreSQL started: postgres://zaiko@127.0.0.1:${Port}/zaiko"
        [void](Show-Status)
    }

    'stop' {
        & $wslPath -d $Distro -u root -- pg_ctlcluster --skip-systemctl-redirect 18 main stop 2>$null
        $keeper = Get-KeeperProcess
        if ($null -ne $keeper) {
            Stop-Process -Id (Get-KeeperId -Keeper $keeper) -Force
        }
        if (Test-Path -LiteralPath $statePath) {
            Remove-Item -LiteralPath $statePath -Force
        }
        Write-Host 'The project PostgreSQL service has been stopped.'
    }

    'status' {
        if (-not (Show-Status)) {
            exit 1
        }
    }
}
