[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$installRoot = Split-Path -Parent $PSScriptRoot
$desktopShortcut = Join-Path ([Environment]::GetFolderPath("Desktop")) "AiHelper.lnk"
$startMenuDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\AiHelper"

Get-Process -ErrorAction SilentlyContinue | Where-Object {
    $_.Path -eq (Join-Path $installRoot "AiHelper.exe")
} | Stop-Process -Force -ErrorAction SilentlyContinue

if (Test-Path $desktopShortcut) {
    Remove-Item -LiteralPath $desktopShortcut -Force
}
if (Test-Path $startMenuDir) {
    Remove-Item -LiteralPath $startMenuDir -Recurse -Force
}

Start-Sleep -Milliseconds 500
Remove-Item -LiteralPath $installRoot -Recurse -Force
