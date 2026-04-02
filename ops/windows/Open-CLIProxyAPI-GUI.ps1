param(
    [int]$Port = 8317,
    [string]$Url,
    [int]$StartupTimeoutSec = 60
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'Common.ps1')
$paths = Get-CLIProxyAPIPaths

if ([string]::IsNullOrWhiteSpace($Url)) {
    $Url = "http://127.0.0.1:$Port/management.html"
}
$readyUrl = "http://127.0.0.1:$Port/"
$lastErrorPath = Join-Path $paths.LogsDir 'open-gui-last-error.txt'

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

function Test-Http200 {
    param([Parameter(Mandatory = $true)][string]$Uri)

    try {
        $r = Invoke-WebRequestCompat -Uri $Uri -TimeoutMs 500
        return ($null -ne $r -and $r.StatusCode -eq 200)
    } catch {
        return $false
    }
}

function Write-LastErrorLog {
    param(
        [Parameter(Mandatory = $true)][string]$Text
    )

    try {
        $dir = Split-Path -Parent $lastErrorPath
        if ($dir -and -not (Test-Path $dir)) {
            New-Item -ItemType Directory -Force -Path $dir | Out-Null
        }
        $enc = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($lastErrorPath, $Text, $enc)
    } catch {
        # Best-effort only: never block GUI open flow due to logging.
    }
}

function Show-UiMessage {
    param(
        [Parameter(Mandatory = $true)][string]$Title,
        [Parameter(Mandatory = $true)][string]$Message,
        [ValidateSet('Info', 'Warning', 'Error')][string]$Level = 'Info'
    )

    try {
        Add-Type -AssemblyName System.Windows.Forms | Out-Null
        $icon = [System.Windows.Forms.MessageBoxIcon]::$Level
        [void][System.Windows.Forms.MessageBox]::Show($Message, $Title, [System.Windows.Forms.MessageBoxButtons]::OK, $icon)
        return
    } catch {}

    try {
        $wsh = New-Object -ComObject WScript.Shell
        $iconCode = switch ($Level) { 'Error' { 16 } 'Warning' { 48 } Default { 64 } }
        [void]$wsh.Popup($Message, 0, $Title, $iconCode)
    } catch {}
}

function Get-AppPathExe {
    param([Parameter(Mandatory = $true)][string]$ExeName)

    $k = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\$ExeName"
    if (Test-Path $k) {
        try {
            $p = Get-ItemProperty -Path $k -ErrorAction Stop
            $exe = [string]$p.'(default)'
            if ($exe -and (Test-Path $exe)) { return $exe }
        } catch {}
    }

    try {
        $cmd = Get-Command ($ExeName -replace '\.exe$','') -ErrorAction Stop
        if ($cmd.Path -and (Test-Path $cmd.Path)) { return $cmd.Path }
    } catch {}

    return $null
}

function Get-EdgeLastUsedProfileDirectory {
    $localState = Join-Path $env:LOCALAPPDATA 'Microsoft\Edge\User Data\Local State'
    if (-not (Test-Path $localState)) { return $null }

    try {
        $raw = Get-Content -Raw -Encoding UTF8 $localState
        $j = $raw | ConvertFrom-Json -ErrorAction Stop
        $lastUsed = [string]$j.profile.last_used
        if ([string]::IsNullOrWhiteSpace($lastUsed)) { return $null }
        return $lastUsed
    } catch {
        return $null
    }
}

function Get-EdgeProcessesForTargetUrl {
    param([Parameter(Mandatory = $true)][string]$TargetUrl)

    $matched = New-Object 'System.Collections.Generic.List[System.Diagnostics.Process]'
    $seen = New-Object 'System.Collections.Generic.HashSet[int]'

    $processRows = @(Get-CimInstance Win32_Process -Filter "name = 'msedge.exe'" -ErrorAction SilentlyContinue)
    foreach ($row in $processRows) {
        $commandLine = [string]$row.CommandLine
        if ([string]::IsNullOrWhiteSpace($commandLine) -or -not $commandLine.Contains($TargetUrl)) {
            continue
        }

        $procId = [int]$row.ProcessId
        if (-not $seen.Add($procId)) {
            continue
        }

        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        if ($null -ne $proc) {
            $matched.Add($proc) | Out-Null
        }
    }

    return @($matched | Sort-Object StartTime -Descending)
}

function Add-LauncherAutoLoginParameters {
    param([Parameter(Mandatory = $true)][string]$TargetUrl)

    if ($TargetUrl -notmatch '/management\.html($|[#?])') {
        return $TargetUrl
    }

    $launcherKey = ''
    if (Test-Path $paths.ManagementKeyPath) {
        try {
            $launcherKey = (Get-Content -Raw -Encoding UTF8 $paths.ManagementKeyPath).Trim()
        } catch {
            $launcherKey = ''
        }
    }

    $parts = $TargetUrl.Split('#', 2)
    $baseUrl = $parts[0]
    $hash = if ($parts.Length -gt 1) { $parts[1] } else { '' }

    $routePath = '/login'
    $routeQuery = ''
    if (-not [string]::IsNullOrWhiteSpace($hash) -and $hash.StartsWith('/')) {
        $hashParts = $hash.Split('?', 2)
        if (-not [string]::IsNullOrWhiteSpace($hashParts[0])) {
            $routePath = $hashParts[0]
        }
        if ($hashParts.Length -gt 1) {
            $routeQuery = $hashParts[1]
        }
    }

    if (-not [string]::IsNullOrWhiteSpace($routeQuery)) {
        $routeQuery = [regex]::Replace($routeQuery, '(^|&)(launcher_auto_login|launcher-auto-login|launcher_key)=[^&]*', '$1')
        $routeQuery = [regex]::Replace($routeQuery, '&{2,}', '&').Trim('&')
    }

    $routeParts = @()
    if (-not [string]::IsNullOrWhiteSpace($routeQuery)) {
        $routeParts += $routeQuery
    }
    $routeParts += 'launcher_auto_login=1'

    if (-not [string]::IsNullOrWhiteSpace($launcherKey)) {
        $encodedKey = [System.Uri]::EscapeDataString($launcherKey)
        $routeParts += "launcher_key=$encodedKey"
    }

    $mergedRouteQuery = ($routeParts | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }) -join '&'
    if ([string]::IsNullOrWhiteSpace($mergedRouteQuery)) {
        return "${baseUrl}#${routePath}"
    }

    return "${baseUrl}#${routePath}?${mergedRouteQuery}"
}

function Ensure-User32WindowApi {
    if ('CLIProxyAPI.WindowApi' -as [type]) { return }
    Add-Type -Namespace CLIProxyAPI -Name WindowApi -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll")]
public static extern bool SetForegroundWindow(System.IntPtr hWnd);

[System.Runtime.InteropServices.DllImport("user32.dll")]
public static extern bool ShowWindow(System.IntPtr hWnd, int nCmdShow);

[System.Runtime.InteropServices.DllImport("user32.dll")]
public static extern bool BringWindowToTop(System.IntPtr hWnd);
'@
}

function Find-EdgeWindowForUrl {
    param([Parameter(Mandatory = $true)][string]$Url)

    $exactMatches = @(Get-EdgeProcessesForTargetUrl -TargetUrl $Url)
    foreach ($proc in $exactMatches) {
        if ($proc.MainWindowHandle -ne 0) {
            return $proc
        }
    }

    $needles = @(
        'CLI Proxy API',
        'CLI PROXY API',
        'Management Center',
        'management',
        '管理'
    )
    $procs = @(Get-Process -ErrorAction SilentlyContinue msedge | Where-Object { $_.MainWindowHandle -ne 0 })
    foreach ($p in $procs) {
        $t = [string]$p.MainWindowTitle
        if (-not $t) { continue }
        foreach ($n in $needles) {
            if ($t -like "*$n*") { return $p }
        }
    }
    return $null
}

function Activate-ProcessWindow {
    param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$Process)

    Ensure-User32WindowApi
    $h = [IntPtr]$Process.MainWindowHandle
    if ($h -eq [IntPtr]::Zero) { return $false }
    [void][CLIProxyAPI.WindowApi]::ShowWindow($h, 9)
    [void][CLIProxyAPI.WindowApi]::BringWindowToTop($h)
    return [CLIProxyAPI.WindowApi]::SetForegroundWindow($h)
}

function Open-UrlVisible {
    param([Parameter(Mandatory = $true)][string]$TargetUrl)

    $forceLauncherReload = $TargetUrl -match 'launcher_auto_login=1'

    $existing = Find-EdgeWindowForUrl -Url $TargetUrl
    if ($null -ne $existing -and -not $forceLauncherReload) {
        $activatedExisting = Activate-ProcessWindow -Process $existing
        if ($activatedExisting) { return $true }
    }

    $edge = Get-AppPathExe -ExeName 'msedge.exe'
    if (-not $edge) {
        Start-Process -FilePath 'explorer.exe' -ArgumentList $TargetUrl -ErrorAction Stop | Out-Null
        return $true
    }

    $profileDir = Get-EdgeLastUsedProfileDirectory
    $profileArg = if ($profileDir) { "--profile-directory=`"$profileDir`"" } else { $null }

    $appArgs = @()
    if ($profileArg) { $appArgs += $profileArg }
    $appArgs += "--app=$TargetUrl"
    Start-Process -FilePath $edge -ArgumentList $appArgs -ErrorAction Stop | Out-Null

    $activated = $false
    for ($i = 0; $i -lt 75 -and -not $activated; $i++) {
        Start-Sleep -Milliseconds 200
        $p = Find-EdgeWindowForUrl -Url $TargetUrl
        if ($null -ne $p) {
            $activated = Activate-ProcessWindow -Process $p
        }
    }

    if ($activated) { return $true }

    $winArgs = @()
    if ($profileArg) { $winArgs += $profileArg }
    $winArgs += @('--new-window', $TargetUrl)
    Start-Process -FilePath $edge -ArgumentList $winArgs -ErrorAction Stop | Out-Null
    for ($i = 0; $i -lt 75 -and -not $activated; $i++) {
        Start-Sleep -Milliseconds 200
        $p = Find-EdgeWindowForUrl -Url $TargetUrl
        if ($null -ne $p) {
            $activated = Activate-ProcessWindow -Process $p
        }
    }

    if ($activated) { return $true }

    Start-Process -FilePath 'explorer.exe' -ArgumentList $TargetUrl -ErrorAction Stop | Out-Null
    return $false
}

$startScript = Join-Path $PSScriptRoot 'Start-CLIProxyAPI.ps1'
if (-not (Test-Path $startScript)) {
    throw "Missing script: $startScript"
}

try {
    $Url = Add-LauncherAutoLoginParameters -TargetUrl $Url

    if (-not (Test-Http200 -Uri $readyUrl)) {
        & $startScript -Port $Port -StartupTimeoutSec $StartupTimeoutSec
    }

    $deadline = (Get-Date).AddSeconds($StartupTimeoutSec)
    while ((Get-Date) -lt $deadline -and -not (Test-Http200 -Uri $readyUrl)) {
        Start-Sleep -Milliseconds 200
    }

    if (-not (Test-Http200 -Uri $readyUrl)) {
        throw "CLIProxyAPI is not ready at: $readyUrl"
    }

    $ok = Open-UrlVisible -TargetUrl $Url
    if (-not $ok) {
        Write-LastErrorLog -Text ("[{0}] WARN: Open-UrlVisible did not confirm activation.`r`nURL: {1}`r`nPort: {2}`r`nUser: {3}`r`nComputer: {4}`r`n" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Url, $Port, $env:USERNAME, $env:COMPUTERNAME)
        Show-UiMessage -Title 'CLIProxyAPI - Open GUI' -Level Warning -Message "已尝试打开管理界面，但未能确认窗口被激活。`n`n请检查：Edge 是否在后台/最小化/其它桌面。`nURL: $Url"
    }
} catch {
    $msg = $_.Exception.Message
    $stack = $_.ScriptStackTrace
    Write-LastErrorLog -Text ("[{0}] ERROR: {1}`r`n`r`nSTACK:`r`n{2}`r`n`r`nURL: {3}`r`nPort: {4}`r`nUser: {5}`r`nComputer: {6}`r`n" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $msg, $stack, $Url, $Port, $env:USERNAME, $env:COMPUTERNAME)
    Show-UiMessage -Title 'CLIProxyAPI - Open GUI 失败' -Level Error -Message ("错误: {0}`n`n堆栈:`n{1}`n`nURL: {2}" -f $msg, $stack, $Url)
    throw
}
