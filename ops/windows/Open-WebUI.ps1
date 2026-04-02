Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$url = 'http://127.0.0.1:8317/management.html'

& (Join-Path $PSScriptRoot 'Start-CLIProxyAPI.ps1') | Out-Null

Start-Process -FilePath $url | Out-Null
