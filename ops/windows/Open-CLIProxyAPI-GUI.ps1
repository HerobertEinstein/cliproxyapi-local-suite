param(
    [int]$Port = 8317,
    [string]$Url
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Url)) {
    $Url = "http://127.0.0.1:$Port/management.html"
}

$startScript = Join-Path $PSScriptRoot 'Start-CLIProxyAPI.ps1'
if (-not (Test-Path $startScript)) {
    throw "Missing script: $startScript"
}

& $startScript -Port $Port | Out-Null
Start-Process -FilePath $Url | Out-Null

