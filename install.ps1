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

function Add-AgentctlPath {
  param([string]$Dir)
  if (-not $Dir) { return }
  $expanded = [Environment]::ExpandEnvironmentVariables($Dir)
  if (-not (Test-Path $expanded)) { return }

  $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $userParts = @()
  if ($UserPath) { $userParts = $UserPath -split ';' | Where-Object { $_ } }
  $inUserPath = $false
  foreach ($part in $userParts) {
    if ($part.TrimEnd('\\') -ieq $expanded.TrimEnd('\\')) { $inUserPath = $true; break }
  }
  if (-not $inUserPath) {
    $newUserPath = if ($UserPath) { "$UserPath;$expanded" } else { $expanded }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    Write-Host "Added $expanded to your user PATH."
  }

  $processParts = @()
  if ($env:PATH) { $processParts = $env:PATH -split ';' | Where-Object { $_ } }
  $inProcessPath = $false
  foreach ($part in $processParts) {
    if ($part.TrimEnd('\\') -ieq $expanded.TrimEnd('\\')) { $inProcessPath = $true; break }
  }
  if (-not $inProcessPath) {
    $env:PATH = "$expanded;$env:PATH"
    Write-Host "Added $expanded to this PowerShell session PATH."
  }
}

Add-AgentctlPath $InstallDir
Add-AgentctlPath (Join-Path $env:APPDATA "npm")
Add-AgentctlPath (Join-Path $env:USERPROFILE ".local\bin")

& $OutFile status
