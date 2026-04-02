Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'Common.ps1')

$stopped = Stop-CLIProxyAPIProcess
if ($stopped) {
    Write-Host 'Stopped.'
} else {
    Write-Host 'Not running.'
}
