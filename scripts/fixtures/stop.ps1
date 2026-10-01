$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$cache = Join-Path $root '.fixture-cache'
$statePath = Join-Path $cache 'state.json'
if (!(Test-Path -LiteralPath $statePath)) { return }
$state = Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
foreach ($path in @($state.registryPaths)) {
  if ($path -and $path -match '^HKCU:\\Software\\(Google\\Chrome|Chromium)\\NativeMessagingHosts\\io\.legacylens\.host$' -and (Test-Path $path)) {
    $value = (Get-Item $path).GetValue('')
    if ($value -eq $state.manifestPath) { Remove-Item -LiteralPath $path -Force }
  }
}
if ($state.manifestPath -and [IO.Path]::GetFullPath([string]$state.manifestPath) -eq [IO.Path]::GetFullPath((Join-Path $cache 'io.legacylens.host.json'))) { Remove-Item -LiteralPath $state.manifestPath -Force -ErrorAction SilentlyContinue }
if ($state.runtime -and $state.wildflyPath) {
  $wildflyRoot = Split-Path -Path (Split-Path -Path ([string]$state.wildflyPath) -Parent) -Parent
  $agentArg = Join-Path ([string]$state.runtime) 'agent.jar'
  Get-CimInstance Win32_Process -Filter "name='java.exe'" | Where-Object { $_.CommandLine -and $_.CommandLine.Contains($agentArg) -and $_.CommandLine.Contains($wildflyRoot) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
}
foreach ($entry in @(@($state.wildflyPid,$state.wildflyPath),@($state.corePid,$state.corePath))) {
  if (!$entry[0] -or !$entry[1]) { continue }
  $process = Get-CimInstance Win32_Process -Filter "ProcessId=$($entry[0])" -ErrorAction SilentlyContinue
  if ($process -and $process.CommandLine -and $process.CommandLine.Contains([string]$entry[1])) { Stop-Process -Id $entry[0] -Force }
}
if ($state.containerName -match '^legacylens-fixture-[0-9a-f]{12}$') {
  $id = & docker inspect -f '{{.Id}}' $state.containerName 2>$null
  if ($LASTEXITCODE -eq 0 -and $id -eq $state.containerId) { & docker stop $state.containerName | Out-Null }
}
Remove-Item -LiteralPath $statePath -Force
Remove-Item -LiteralPath (Join-Path $cache 'agent.properties') -Force -ErrorAction SilentlyContinue
if ($state.runtime -and $state.containerName -match '^legacylens-fixture-[0-9a-f]{12}$') {
  $expected = Join-Path $env:TEMP $state.containerName
  if ([IO.Path]::GetFullPath($expected) -eq [IO.Path]::GetFullPath([string]$state.runtime)) {
    Remove-Item -LiteralPath (Join-Path $expected 'agent.properties'),(Join-Path $expected 'agent.jar') -Force -ErrorAction SilentlyContinue
    try { Remove-Item -LiteralPath $expected -ErrorAction Stop } catch { }
  }
}
