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
  $wildflyRoot = if ($state.wildflyRoot) { [IO.Path]::GetFullPath([string]$state.wildflyRoot) } else { Split-Path -Path (Split-Path -Path ([string]$state.wildflyPath) -Parent) -Parent }
  $agentArg = Join-Path ([string]$state.runtime) 'agent.jar'
  Get-CimInstance Win32_Process -Filter "name='java.exe'" | Where-Object { $_.CommandLine -and $_.CommandLine.Contains($agentArg) -and $_.CommandLine.Contains($wildflyRoot) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
}
foreach ($entry in @(@($state.wildflyPid,$state.wildflyPath),@($state.corePid,$state.corePath))) {
  if (!$entry[0] -or !$entry[1]) { continue }
  $process = Get-CimInstance Win32_Process -Filter "ProcessId=$($entry[0])" -ErrorAction SilentlyContinue
  if ($process -and $process.CommandLine -and $process.CommandLine.Contains([string]$entry[1])) { Stop-Process -Id $entry[0] -Force }
}
if ($state.mysqlPid -and $state.mysqlPath) {
  $expectedMysqlPaths = @(
    [IO.Path]::GetFullPath((Join-Path $cache 'mysql57\mysql-5.7.44-winx64\bin\mysqld.exe')),
    [IO.Path]::GetFullPath((Join-Path $cache 'mysql84\mysql-8.4.0-winx64\bin\mysqld.exe'))
  )
  $expectedMysqlPath = $expectedMysqlPaths | Where-Object { [IO.Path]::GetFullPath([string]$state.mysqlPath) -eq $_ } | Select-Object -First 1
  if ($expectedMysqlPath) {
    $mysqlProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$($state.mysqlPid)" -ErrorAction SilentlyContinue
    if ($mysqlProcess -and [IO.Path]::GetFullPath([string]$mysqlProcess.ExecutablePath) -eq $expectedMysqlPath) {
      $mysqlHome = Split-Path -Path (Split-Path -Path $expectedMysqlPath -Parent) -Parent
      $mysqladmin = Join-Path $mysqlHome 'bin\mysqladmin.exe'
      & $mysqladmin --no-defaults --protocol=tcp --host=127.0.0.1 "--port=$($state.mysqlPort)" --user=root --password=fixture-root shutdown 2>$null | Out-Null
      for ($i=0; $i -lt 20; $i++) {
        if (!(Get-CimInstance Win32_Process -Filter "ProcessId=$($state.mysqlPid)" -ErrorAction SilentlyContinue)) { break }
        Start-Sleep -Milliseconds 250
      }
      if (Get-CimInstance Win32_Process -Filter "ProcessId=$($state.mysqlPid)" -ErrorAction SilentlyContinue) { Stop-Process -Id $state.mysqlPid -Force }
    }
  }
}
Remove-Item -LiteralPath $statePath -Force
Remove-Item -LiteralPath (Join-Path $cache 'agent.properties') -Force -ErrorAction SilentlyContinue
if ($state.runtime -and $state.fixtureId -match '^legacylens-fixture-[0-9a-f]{12}$') {
  $expected = Join-Path $env:TEMP $state.fixtureId
  if ([IO.Path]::GetFullPath($expected) -eq [IO.Path]::GetFullPath([string]$state.runtime)) {
    Remove-Item -LiteralPath (Join-Path $expected 'agent.properties'),(Join-Path $expected 'agent.jar') -Force -ErrorAction SilentlyContinue
    try { Remove-Item -LiteralPath $expected -ErrorAction Stop } catch { }
  }
}
$expectedCache = [IO.Path]::GetFullPath($cache) + [IO.Path]::DirectorySeparatorChar
if ($state.mysqlData) {
  $expectedMysqlData = [IO.Path]::GetFullPath([string]$state.mysqlData)
  if ($expectedMysqlData.StartsWith($expectedCache,[StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $expectedMysqlData) -match '^mysql-data-[0-9a-f]{12}$' -and !(Get-CimInstance Win32_Process -Filter "ProcessId=$($state.mysqlPid)" -ErrorAction SilentlyContinue)) {
    Remove-Item -LiteralPath $expectedMysqlData -Recurse -Force -ErrorAction SilentlyContinue
  }
}
