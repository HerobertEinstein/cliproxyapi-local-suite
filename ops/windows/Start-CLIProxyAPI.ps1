param(
    [int]$Port = 8317,
    [int]$StartupTimeoutSec = 20
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'Common.ps1')

function Invoke-WebRequestCompat {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [int]$TimeoutMs = 500
    )

    $request = [System.Net.HttpWebRequest]::Create($Uri)
    $request.Method = 'GET'
    $request.Timeout = [Math]::Max(100, $TimeoutMs)
    $request.ReadWriteTimeout = [Math]::Max(100, $TimeoutMs)
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
        if ($null -ne $_.Exception.Response) {
            $response = $_.Exception.Response
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
    param([Parameter(Mandatory = $true)][string]$Uri)

    try {
        $result = Invoke-WebRequestCompat -Uri $Uri -TimeoutMs 500
        return ($null -ne $result -and $result.StatusCode -eq 200)
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

$readyUrl = "http://127.0.0.1:$Port/"
if (Test-CLIProxyAPIHttpReady -Uri $readyUrl) {
    Write-Host "CLIProxyAPI is already serving at $readyUrl"
    return
}

$running = @(Get-CLIProxyAPIProcess)
if ($running.Count -gt 0) {
    $deadline = (Get-Date).AddSeconds($StartupTimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-CLIProxyAPIHttpReady -Uri $readyUrl) {
            Write-Host "CLIProxyAPI is ready."
            return
        }
        Start-Sleep -Milliseconds 200
    }

    throw "CLIProxyAPI process exists but HTTP is not ready: $readyUrl"
}

$outLog = Join-Path $paths.LogsDir 'cpa.out.log'
$errLog = Join-Path $paths.LogsDir 'cpa.err.log'

Start-Process `
    -FilePath $paths.ExePath `
    -ArgumentList @('-config', $paths.ConfigPath) `
    -WorkingDirectory $paths.RuntimeDir `
    -WindowStyle Hidden `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog | Out-Null

$deadline2 = (Get-Date).AddSeconds($StartupTimeoutSec)
while ((Get-Date) -lt $deadline2) {
    if ((@(Get-CLIProxyAPIProcess)).Count -gt 0 -and (Test-CLIProxyAPIHttpReady -Uri $readyUrl)) {
        Write-Host "CLIProxyAPI started."
        return
    }
    Start-Sleep -Milliseconds 200
}

throw "CLIProxyAPI start timeout. Check logs: $outLog / $errLog"

