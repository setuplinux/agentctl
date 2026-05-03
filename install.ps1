$ErrorActionPreference = "Stop"

$Repo = if ($env:AGENTCTL_REPO) { $env:AGENTCTL_REPO } else { "setuplinux/agentctl" }
$Version = if ($env:AGENTCTL_VERSION) { $env:AGENTCTL_VERSION } else { "latest" }
$InstallDir = if ($env:AGENTCTL_INSTALL_DIR) { $env:AGENTCTL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "agentctl" }

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$Asset = "agentctl-windows-$Arch.exe"
$Base = "https://github.com/$Repo/releases"
if ($Version -eq "latest") {
  $Url = "$Base/latest/download/$Asset"
} else {
  $Url = "$Base/download/$Version/$Asset"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$OutFile = Join-Path $InstallDir "agentctl.exe"

Write-Host "Downloading $Asset from $Repo..."
Invoke-WebRequest -Uri $Url -OutFile $OutFile

Write-Host "Installed agentctl to $OutFile"
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
  Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use agentctl directly."
}

& $OutFile status
