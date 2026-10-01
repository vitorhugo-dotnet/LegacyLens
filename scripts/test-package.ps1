[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$PackageDirectory)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$package = (Resolve-Path $PackageDirectory).Path
$exe = Join-Path $package 'legacylens.exe'
if (-not (Test-Path $exe)) { throw 'Packaged core executable is missing.' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ("LegacyLens smoke " + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
$previousPath = $env:PATH
$previousAppData = $env:APPDATA
$server = $null
try {
  & (Join-Path $PSScriptRoot 'test-agent-smoke.ps1') -AgentJar (Join-Path $package 'agent/legacylens-agent.jar') -AgentSmokeClassPath (Join-Path (Split-Path $PSScriptRoot -Parent) 'java/agent/target/test-classes')
  $env:PATH = "$env:SystemRoot\System32;$env:SystemRoot"
  $env:APPDATA = Join-Path $temp 'AppData'
  if (Get-Command go,node -ErrorAction SilentlyContinue) { throw 'Go or Node remains available on the smoke-test PATH.' }
  $server = Start-Process -FilePath $exe -ArgumentList 'serve' -PassThru -WindowStyle Hidden -WorkingDirectory $package
  $healthy = $false
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    if ($server.HasExited) { throw 'Packaged core exited before becoming healthy.' }
    & $exe status *> $null
    if ($LASTEXITCODE -eq 0) { $healthy = $true; break }
    Start-Sleep -Milliseconds 500
  }
  if (-not $healthy) { throw 'Packaged core did not become healthy without Go or Node on PATH.' }
} finally {
  if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
  $env:PATH = $previousPath
  if ($null -eq $previousAppData) { Remove-Item Env:APPDATA -ErrorAction SilentlyContinue } else { $env:APPDATA = $previousAppData }
  Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
