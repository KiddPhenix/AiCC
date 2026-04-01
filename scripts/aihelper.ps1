[CmdletBinding()]
param(
    [ValidateSet("start", "stop", "restart", "status", "logs", "debug")]
    [string]$Action = "start"
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$runtimeDir = Join-Path $root "data\runtime"
$binDir = Join-Path $runtimeDir "bin"
$pidFile = Join-Path $runtimeDir "processes.json"
$webPort = 8090
$webUrl = "http://127.0.0.1:$webPort"

function Ensure-RuntimeDir {
    New-Item -ItemType Directory -Force -Path $runtimeDir | Out-Null
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
}

function Read-ProcessInfo {
    if (-not (Test-Path $pidFile)) { return $null }
    try { return Get-Content $pidFile -Raw | ConvertFrom-Json } catch { return $null }
}

function Remove-ProcessInfo {
    if (Test-Path $pidFile) { Remove-Item -LiteralPath $pidFile -Force }
}

function Get-AliveProcess {
    param([int]$Id)
    if (-not $Id) { return $null }
    try { return Get-Process -Id $Id -ErrorAction Stop } catch { return $null }
}

function Stop-ManagedProcesses {
    $info = Read-ProcessInfo
    if ($null -ne $info) {
        $proc = Get-AliveProcess -Id $info.app.pid
        if ($null -ne $proc) {
            Stop-Process -Id $proc.Id -Force
            Write-Host "已停止应用进程 PID=$($proc.Id)"
        }
    }

    $listener = Get-NetTCPConnection -State Listen -LocalPort $webPort -ErrorAction SilentlyContinue
    if ($null -ne $listener) {
        $portProc = Get-AliveProcess -Id $listener.OwningProcess
        if ($null -ne $portProc) {
            Stop-Process -Id $portProc.Id -Force
            Write-Host "已停止占用 $webPort 端口的进程 PID=$($portProc.Id)"
        }
    }

    Remove-ProcessInfo
}

function Wait-ForWeb {
    $deadline = (Get-Date).AddSeconds(20)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri $webUrl -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) { return $true }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    return $false
}

function Start-ManagedProcesses {
    Ensure-RuntimeDir
    Stop-ManagedProcesses

    $appExe = Join-Path $binDir "aihelper.exe"
    $appLog = Join-Path $runtimeDir "app.out.log"
    $appErrLog = Join-Path $runtimeDir "app.err.log"

    & go build -ldflags "-H=windowsgui" -o $appExe cmd/desktop/main.go

    $envMap = @{
        AIHELPER_ADDR = ":$webPort"
        AIHELPER_DB_PATH = "data/aihelper.db"
        AIHELPER_CONFIG_PATH = "config/app.json"
    }

    $appProcess = Start-Process -FilePath $appExe -WorkingDirectory $root -PassThru -WindowStyle Hidden -RedirectStandardOutput $appLog -RedirectStandardError $appErrLog -Environment $envMap

    @{
        started_at = (Get-Date).ToString("s")
        app = @{
            pid = $appProcess.Id
            exe = $appExe
            log = $appLog
            err_log = $appErrLog
            port = $webPort
        }
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $pidFile -Encoding UTF8

    if (Wait-ForWeb) {
        Write-Host "已启动桌面应用: $webUrl"
        return
    }

    Write-Host "应用启动超时，请查看日志：$appErrLog"
}

function Show-Status {
    $info = Read-ProcessInfo
    if ($null -eq $info) {
        Write-Host "当前没有受管进程。"
        return
    }
    $proc = Get-AliveProcess -Id $info.app.pid
    [pscustomobject]@{
        AppPID     = $info.app.pid
        Running    = $null -ne $proc
        WebURL     = $webUrl
        AppLog     = $info.app.log
        AppErrLog  = $info.app.err_log
    } | Format-List
}

function Show-Logs {
    Ensure-RuntimeDir
    $appLog = Join-Path $runtimeDir "app.out.log"
    $appErrLog = Join-Path $runtimeDir "app.err.log"
    foreach ($path in @($appLog, $appErrLog)) {
        if (-not (Test-Path $path)) { New-Item -ItemType File -Path $path -Force | Out-Null }
    }
    Write-Host "正在实时输出日志，按 Ctrl+C 结束查看。"
    Get-Content -Path $appLog, $appErrLog -Wait -Tail 50
}

function Start-DebugMode {
    Start-ManagedProcesses
    Show-Logs
}

switch ($Action) {
    "start"   { Start-ManagedProcesses }
    "stop"    { Stop-ManagedProcesses }
    "restart" { Stop-ManagedProcesses; Start-ManagedProcesses }
    "status"  { Show-Status }
    "logs"    { Show-Logs }
    "debug"   { Start-DebugMode }
}
