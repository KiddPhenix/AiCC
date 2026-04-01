[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$extractRoot = Split-Path -Parent $PSScriptRoot
$appSource = Join-Path $extractRoot "app"
$scriptsSource = Join-Path $extractRoot "scripts"
$installRoot = Join-Path $env:LOCALAPPDATA "AiHelper"
$desktopShortcut = Join-Path ([Environment]::GetFolderPath("Desktop")) "AiHelper.lnk"
$startMenuDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\AiHelper"
$startMenuShortcut = Join-Path $startMenuDir "AiHelper.lnk"
$uninstallShortcut = Join-Path $startMenuDir "卸载 AiHelper.lnk"
$targetExe = Join-Path $installRoot "AiHelper.exe"
$uninstallScript = Join-Path $installRoot "scripts\uninstall.ps1"

function New-Shortcut {
    param(
        [string]$Path,
        [string]$TargetPath,
        [string]$WorkingDirectory,
        [string]$Arguments = ""
    )

    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($Path)
    $shortcut.TargetPath = $TargetPath
    $shortcut.WorkingDirectory = $WorkingDirectory
    if ($Arguments) {
        $shortcut.Arguments = $Arguments
    }
    $shortcut.IconLocation = "$TargetPath,0"
    $shortcut.Save()
}

if (Test-Path $installRoot) {
    Remove-Item -LiteralPath $installRoot -Recurse -Force
}

New-Item -ItemType Directory -Path $installRoot -Force | Out-Null
Copy-Item -Path (Join-Path $appSource "*") -Destination $installRoot -Recurse -Force
Copy-Item -Path (Join-Path $scriptsSource "*") -Destination (Join-Path $installRoot "scripts") -Recurse -Force

$dataDir = Join-Path $installRoot "data"
New-Item -ItemType Directory -Path $dataDir, (Join-Path $dataDir "runtime"), (Join-Path $dataDir "examples") -Force | Out-Null

New-Item -ItemType Directory -Path $startMenuDir -Force | Out-Null
New-Shortcut -Path $desktopShortcut -TargetPath $targetExe -WorkingDirectory $installRoot
New-Shortcut -Path $startMenuShortcut -TargetPath $targetExe -WorkingDirectory $installRoot
New-Shortcut -Path $uninstallShortcut -TargetPath "powershell.exe" -WorkingDirectory $installRoot -Arguments "-ExecutionPolicy Bypass -File `"$uninstallScript`""

Start-Process -FilePath $targetExe -WorkingDirectory $installRoot -WindowStyle Hidden
