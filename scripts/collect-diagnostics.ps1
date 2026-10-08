[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$OutputPath,
  [ValidatePattern('^(unknown|[0-9a-fA-F]{7,40})$')][string]$Commit = 'unknown',
  [ValidatePattern('^(unknown|([0-9]+\.){1,3}[0-9A-Za-z.+-]+)$')][string]$Release = 'unknown'
)
$ErrorActionPreference = 'Stop'
$os = Get-CimInstance -ClassName Win32_OperatingSystem
$safeOutput = [System.IO.Path]::GetFullPath($OutputPath)
$parent = Split-Path -Parent $safeOutput
if (-not (Test-Path -LiteralPath $parent)) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
$scenarioIds = @('install','uninstall','intellij-integration','interaction-without-request','complete-capture','static-impact')
$result = [ordered]@{
  schemaVersion = 1
  collectedAt = [DateTime]::UtcNow.ToString('o')
  commit = $Commit.ToLowerInvariant()
  release = $Release
  platform = [ordered]@{
    name = [string]$os.Caption
    architecture = if ([Environment]::Is64BitOperatingSystem) { 'x64' } else { 'x86' }
    version = [string]$os.Version
    build = [string]$os.BuildNumber
    powershell = [string]$PSVersionTable.PSVersion
  }
  stack = [ordered]@{
    javaRuntime = $null
    applicationServer = $null
    faces = $null
    primeFaces = $null
    database = $null
    jdbcDriver = $null
  }
  indexRevision = $null
  deploymentRevision = $null
  scenarios = @($scenarioIds | ForEach-Object { [ordered]@{ id=$_; status='not-run' } })
  collection = [ordered]@{
    sourceFilesIncluded = $false
    logsIncluded = $false
    tracesIncluded = $false
    environmentVariablesIncluded = $false
    credentialsIncluded = $false
  }
}
$json = $result | ConvertTo-Json -Depth 8
Set-Content -LiteralPath $safeOutput -Value $json -Encoding utf8
Write-Output $safeOutput
