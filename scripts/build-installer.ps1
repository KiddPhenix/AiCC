[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $root "dist"
$stageRoot = Join-Path $distRoot "stage"
$packageRoot = Join-Path $stageRoot "package"
$buildRoot = Join-Path $distRoot "build"
$installerDir = Join-Path $root "scripts\installer"
$desktopExe = Join-Path $buildRoot "AiHelper.exe"
$portableZip = Join-Path $distRoot "AiHelper-portable.zip"
$installerZip = Join-Path $distRoot "AiHelper-installer.zip"
$installerExe = Join-Path $distRoot "AiHelper-Setup.exe"
$sedPath = Join-Path $buildRoot "AiHelper-Setup.sed"

function Reset-Dir {
    param([string]$Path)
    if (Test-Path $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
}

function Copy-Tree {
    param(
        [string]$Source,
        [string]$Destination
    )
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    Copy-Item -Path (Join-Path $Source "*") -Destination $Destination -Recurse -Force
}

Reset-Dir $distRoot
New-Item -ItemType Directory -Path $stageRoot, $packageRoot, $buildRoot -Force | Out-Null

Push-Location $root
try {
    & go build -ldflags "-H=windowsgui" -o $desktopExe .\cmd\desktop\main.go
}
finally {
    Pop-Location
}

New-Item -ItemType Directory -Path (Join-Path $packageRoot "app"), (Join-Path $packageRoot "scripts") -Force | Out-Null

Copy-Item -LiteralPath $desktopExe -Destination (Join-Path $packageRoot "app\AiHelper.exe") -Force
Copy-Tree -Source (Join-Path $root "web") -Destination (Join-Path $packageRoot "app\web")
New-Item -ItemType Directory -Path (Join-Path $packageRoot "app\config") -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $root "config\app.example.json") -Destination (Join-Path $packageRoot "app\config\app.example.json") -Force
@'
{
  "ai": {
    "base_url": "https://api.openai.com/v1",
    "api_key": "",
    "model": "gpt-4o-mini"
  },
  "capture": {
    "auto_analyze": false
  }
}
'@ | Set-Content -LiteralPath (Join-Path $packageRoot "app\config\app.json") -Encoding UTF8
Copy-Item -LiteralPath (Join-Path $root "README.md") -Destination (Join-Path $packageRoot "app\README.md") -Force
Copy-Item -LiteralPath (Join-Path $installerDir "install.ps1") -Destination (Join-Path $packageRoot "scripts\install.ps1") -Force
Copy-Item -LiteralPath (Join-Path $installerDir "uninstall.ps1") -Destination (Join-Path $packageRoot "scripts\uninstall.ps1") -Force

Compress-Archive -Path (Join-Path $packageRoot "*") -DestinationPath $portableZip -Force
Compress-Archive -Path (Join-Path $packageRoot "*") -DestinationPath $installerZip -Force

$sedContent = @"
[Version]
Class=IEXPRESS
SEDVersion=3
[Options]
PackagePurpose=InstallApp
ShowInstallProgramWindow=1
HideExtractAnimation=0
UseLongFileName=1
InsideCompressed=0
CAB_FixedSize=0
CAB_ResvCodeSigning=0
RebootMode=N
InstallPrompt=
DisplayLicense=
FinishMessage=AiHelper 安装完成。
TargetName=$installerExe
FriendlyName=AiHelper Setup
AppLaunched=powershell.exe -ExecutionPolicy Bypass -File install.ps1
PostInstallCmd=<None>
AdminQuietInstCmd=
UserQuietInstCmd=
SourceFiles=SourceFiles
[SourceFiles]
SourceFiles0=$packageRoot\app
SourceFiles1=$packageRoot\app\web\static
SourceFiles2=$packageRoot\app\config
SourceFiles3=$packageRoot\scripts
[SourceFiles0]
%FILE0%=
%FILE1%=
[SourceFiles1]
%FILE2%=
[SourceFiles2]
%FILE3%=
%FILE4%=
[SourceFiles3]
%FILE5%=
%FILE6%=
[Strings]
FILE0=AiHelper.exe
FILE1=README.md
FILE2=index.html
FILE3=app.example.json
FILE4=app.json
FILE5=install.ps1
FILE6=uninstall.ps1
"@

Set-Content -LiteralPath $sedPath -Value $sedContent -Encoding ASCII

if (Test-Path "$env:SystemRoot\System32\iexpress.exe") {
    & "$env:SystemRoot\System32\iexpress.exe" /N $sedPath | Out-Null
}

Write-Host "便携包: $portableZip"
Write-Host "安装压缩包: $installerZip"
if (Test-Path $installerExe) {
    Write-Host "单文件安装包: $installerExe"
} else {
    Write-Host "单文件安装包未生成，可直接使用安装压缩包。"
}
