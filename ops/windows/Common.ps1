Set-StrictMode -Version Latest

function Get-CLIProxyAPIPaths {
    $repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
    $runtimeRoot = Join-Path $repoRoot 'runtime'
    $configRoot = Join-Path $repoRoot 'config'
    $logsRoot = Join-Path $repoRoot 'logs'

    return [ordered]@{
        RepoRoot          = $repoRoot
        BackendDir        = Join-Path $repoRoot 'backend'
        WebUiDir          = Join-Path $repoRoot 'webui'
        RuntimeDir        = $runtimeRoot
        AppDir            = Join-Path $runtimeRoot 'app'
        ConfigDir         = $configRoot
        LogsDir           = $logsRoot
        ExePath           = Join-Path $runtimeRoot 'app\cpa.exe'
        ExampleConfigPath = Join-Path $repoRoot 'backend\config.example.yaml'
        ConfigPath        = Join-Path $configRoot 'config.local.yaml'
        ManagementKeyPath = Join-Path $configRoot 'management-key.txt'
        ClientApiKeyPath  = Join-Path $configRoot 'client-api-key.txt'
    }
}

function Write-Utf8NoBomFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Content
    )

    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path $parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }

    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Ensure-CLIProxyAPIDirs {
    $paths = Get-CLIProxyAPIPaths
    New-Item -ItemType Directory -Force -Path $paths.RuntimeDir, $paths.AppDir, $paths.ConfigDir, $paths.LogsDir | Out-Null
}

function New-RandomApiKey {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    } finally {
        if ($null -ne $rng) { $rng.Dispose() }
    }

    return ([Convert]::ToBase64String($bytes)).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

function Get-CLIProxyAPIProcess {
    $paths = Get-CLIProxyAPIPaths
    $exe = [System.IO.Path]::GetFullPath($paths.ExePath)
    return Get-Process -Name 'cpa' -ErrorAction SilentlyContinue | Where-Object {
        $null -ne $_.Path -and ([System.IO.Path]::GetFullPath($_.Path) -eq $exe)
    }
}

function Stop-CLIProxyAPIProcess {
    $processes = @(Get-CLIProxyAPIProcess)
    if ($processes.Count -eq 0) {
        return $false
    }

    $processes | Stop-Process -Force
    return $true
}

