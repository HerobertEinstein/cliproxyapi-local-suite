Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'Common.ps1')

Ensure-CLIProxyAPIDirs
$paths = Get-CLIProxyAPIPaths

if (-not (Test-Path $paths.ExampleConfigPath)) {
    throw "Missing example config: $($paths.ExampleConfigPath)"
}

if (-not (Test-Path $paths.ManagementKeyPath)) {
    Write-Utf8NoBomFile -Path $paths.ManagementKeyPath -Content ((New-RandomApiKey) + "`n")
}

if (-not (Test-Path $paths.ClientApiKeyPath)) {
    Write-Utf8NoBomFile -Path $paths.ClientApiKeyPath -Content ((New-RandomApiKey) + "`n")
}

$managementKey = (Get-Content -Raw -Encoding UTF8 $paths.ManagementKeyPath).Trim()
$clientApiKey = (Get-Content -Raw -Encoding UTF8 $paths.ClientApiKeyPath).Trim()
$config = Get-Content -Raw -Encoding UTF8 $paths.ExampleConfigPath

$config = $config -replace '(?m)^host:\s*.*$', 'host: "127.0.0.1"'
$config = $config -replace '(?m)^port:\s*.*$', 'port: 8317'
$config = $config -replace '(?m)^usage-statistics-enabled:\s*.*$', 'usage-statistics-enabled: true'
$config = $config -replace '(?m)^logging-to-file:\s*.*$', 'logging-to-file: true'
$config = $config -replace '(?m)^ws-auth:\s*.*$', 'ws-auth: false'
$config = $config -replace '(?m)^request-retry:\s*.*$', 'request-retry: 3'
$config = $config -replace '(?m)^max-retry-interval:\s*.*$', 'max-retry-interval: 30'
$config = $config -replace '(?ms)^api-keys:\s*\r?\n(?:\s*-\s*".*"\s*(?:\r?\n|$))+', ('api-keys:' + "`n" + '  - "' + $clientApiKey + '"' + "`n")
$config = $config -replace '(?m)^(\s*secret-key:\s*).*$' , ('$1"' + $managementKey + '"')

Write-Utf8NoBomFile -Path $paths.ConfigPath -Content $config
Write-Host "Synced local config: $($paths.ConfigPath)"

