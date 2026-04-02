param(
    [int]$Port = 8317,
    [int]$StartupTimeoutSec = 60
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'Common.ps1')

function Invoke-WebRequestCompat {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [int]$TimeoutMs = 500
    )

    $timeoutMs = [Math]::Max(100, $TimeoutMs)
    $request = [System.Net.HttpWebRequest]::Create($Uri)
    $request.Method = 'GET'
    $request.Timeout = $timeoutMs
    $request.ReadWriteTimeout = $timeoutMs
    $request.KeepAlive = $false
    $request.Proxy = $null

    try {
        $response = [System.Net.HttpWebResponse]$request.GetResponse()
        try {
            return [pscustomobject]@{ StatusCode = [int]$response.StatusCode }
        } finally {
            $response.Close()
        }
    } catch [System.Net.WebException] {
        $response = $_.Exception.Response
        if ($null -ne $response) {
            try {
                return [pscustomobject]@{ StatusCode = [int]$response.StatusCode }
            } finally {
                $response.Close()
            }
        }
        throw
    }
}

function Test-CLIProxyAPIHttpReady {
    param(
        [Parameter(Mandatory = $true)][string]$Uri
    )

    try {
        $r = Invoke-WebRequestCompat -Uri $Uri -TimeoutMs 500
        return ($null -ne $r -and $r.StatusCode -eq 200)
    } catch {
        return $false
    }
}

Ensure-CLIProxyAPIDirs
$paths = Get-CLIProxyAPIPaths

if (-not (Test-Path $paths.ConfigPath)) {
    & (Join-Path $PSScriptRoot 'Sync-CLIProxyAPI-Config.ps1')
}

if (-not (Test-Path $paths.ExePath)) {
    throw "Missing executable: $($paths.ExePath). Build backend first."
}

$displayUrl = "http://127.0.0.1:$Port/management.html"
$readyUrl = "http://127.0.0.1:$Port/"
if (Test-CLIProxyAPIHttpReady -Uri $readyUrl) {
    Write-Host "CLIProxyAPI is already serving $displayUrl"
    return
}

$running = @(Get-CLIProxyAPIProcess)
if ($running.Count -gt 0) {
    Write-Host "CLIProxyAPI process exists (PID: $($running[0].Id)). Waiting for HTTP ready..."
    $deadline = (Get-Date).AddSeconds($StartupTimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-CLIProxyAPIHttpReady -Uri $readyUrl) {
            Write-Host "CLIProxyAPI is ready."
            return
        }
        Start-Sleep -Milliseconds 200
    }

    throw "CLIProxyAPI process exists (PID: $($running[0].Id)) but HTTP is not ready within ${StartupTimeoutSec}s: $readyUrl"
}

if ($null -ne (Get-Command -Name Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
    $tcp = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if ($null -ne $tcp) {
        $listeningPid = ($tcp | Select-Object -First 1 -ExpandProperty OwningProcess)
        $proc = Get-Process -Id $listeningPid -ErrorAction SilentlyContinue
        $name = if ($null -ne $proc) { [string]$proc.ProcessName } else { 'unknown' }

        if ($name -ieq 'cpa') {
            Write-Host "Port $Port is already listened by cpa (PID: $listeningPid). Waiting for HTTP ready..."
            $deadline = (Get-Date).AddSeconds($StartupTimeoutSec)
            while ((Get-Date) -lt $deadline) {
                if (Test-CLIProxyAPIHttpReady -Uri $readyUrl) {
                    Write-Host "CLIProxyAPI is ready."
                    return
                }
                Start-Sleep -Milliseconds 200
            }

            throw "CLIProxyAPI seems running (port $Port listened by PID $listeningPid), but HTTP is not ready within ${StartupTimeoutSec}s: $readyUrl"
        }

        throw "Port $Port is already in use by PID $listeningPid ($name)."
    }
}

$outLog = Join-Path $paths.LogsDir 'cpa.out.log'
$errLog = Join-Path $paths.LogsDir 'cpa.err.log'

Write-Host "Starting CLIProxyAPI -> http://127.0.0.1:$Port"
Start-Process `
    -FilePath $paths.ExePath `
    -ArgumentList @('-config', $paths.ConfigPath) `
    -WorkingDirectory $paths.RuntimeDir `
    -WindowStyle Hidden `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog | Out-Null

$deadline2 = (Get-Date).AddSeconds($StartupTimeoutSec)
$proc2 = @()
while ((Get-Date) -lt $deadline2) {
    $proc2 = @(Get-CLIProxyAPIProcess)
    if ($proc2.Count -gt 0 -and (Test-CLIProxyAPIHttpReady -Uri $readyUrl)) {
        break
    }
    Start-Sleep -Milliseconds 200
}

if ($proc2.Count -eq 0) {
    throw "Failed to start CLIProxyAPI (process not found). Check logs: $outLog / $errLog"
}

if (-not (Test-CLIProxyAPIHttpReady -Uri $readyUrl)) {
    throw "CLIProxyAPI process started (PID: $($proc2[0].Id)) but HTTP not ready within ${StartupTimeoutSec}s: $readyUrl. Check logs: $outLog / $errLog"
}

Write-Host "Started (PID: $($proc2[0].Id))."
