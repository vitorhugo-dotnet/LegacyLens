[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$PackageDirectory)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$package = (Resolve-Path $PackageDirectory).Path
$exe = Join-Path $package 'legacylens.exe'
if (-not (Test-Path $exe)) { throw 'Packaged core executable is missing.' }
$hostExe = Join-Path $package 'legacylens-host.exe'
if (-not (Test-Path $hostExe)) { throw 'Packaged native messaging host executable is missing.' }
$extensionManifest = Join-Path $package 'extension/manifest.json'
if (-not (Test-Path $extensionManifest)) { throw 'Packaged browser extension is missing.' }
$analyzerJar = Join-Path $package 'analyzer/legacylens-analyzer.jar'
if (-not (Test-Path $analyzerJar)) { throw 'Packaged Java analyzer is missing.' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ("LegacyLens smoke " + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
$previousPath = $env:PATH
$previousAppData = $env:APPDATA
$server = $null
try {
  & (Join-Path $PSScriptRoot 'test-agent-smoke.ps1') -AgentJar (Join-Path $package 'agent/legacylens-agent.jar') -AgentSmokeClassPath (Join-Path (Split-Path $PSScriptRoot -Parent) 'java/agent/target/test-classes')
  $psi = [Diagnostics.ProcessStartInfo]::new((Get-Command java).Source)
  $psi.ArgumentList.Add('-jar'); $psi.ArgumentList.Add($analyzerJar)
  $psi.RedirectStandardInput = $true; $psi.RedirectStandardOutput = $true; $psi.UseShellExecute = $false
  $process = [Diagnostics.Process]::Start($psi)
  $process.StandardInput.WriteLine('{"projectId":"smoke","revisionId":"smoke","artifacts":[{"id":"a1","path":"Sample.java","language":"java"}],"sources":{"Sample.java":"class Sample { void run() {} }"}}')
  $process.StandardInput.Close()
  $output = $process.StandardOutput.ReadToEnd(); $process.WaitForExit()
  $lines = @($output.TrimEnd("`r", "`n") -split "`r?`n")
  if ($process.ExitCode -ne 0 -or $lines.Count -ne 1) { throw 'Packaged Java analyzer did not return exactly one JSONL result.' }
  $analyzerResult = $lines[0] | ConvertFrom-Json
  if (@($analyzerResult.symbols).Count -lt 1 -or @($analyzerResult.PSObject.Properties.Name | Where-Object { $_ -in @('symbols','relations','evidence','diagnostics') }).Count -ne 4) { throw 'Packaged Java analyzer returned an invalid AnalysisResult.' }
  $env:PATH = "$env:SystemRoot\System32;$env:SystemRoot"
  $env:APPDATA = Join-Path $temp 'AppData'
  if (Get-Command go,node,codeql -ErrorAction SilentlyContinue) { throw 'Go, Node, or CodeQL remains available on the smoke-test PATH.' }
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
