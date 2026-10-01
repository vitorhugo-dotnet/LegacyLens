[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$AgentJar,
  [Parameter(Mandatory=$true)][string]$AgentSmokeClassPath
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$jar = (Resolve-Path $AgentJar).Path
$classes = (Resolve-Path $AgentSmokeClassPath).Path
$configPath = Join-Path ([IO.Path]::GetTempPath()) ("legacylens-agent-smoke-" + [guid]::NewGuid().ToString('N') + '.properties')
$properties = @(
  'endpoint=http://127.0.0.1:1/v1/events'
  'token=smoke-token-0123456789'
  'projectId=smoke-project'
  'producerId=smoke-producer'
  'packages=sample.app'
)
try {
  Set-Content -LiteralPath $configPath -Value $properties -Encoding ascii
  & java "-javaagent:$jar=config=$configPath" -cp $classes io.legacylens.agent.AgentSmokeMain
  if ($LASTEXITCODE -ne 0) { throw 'Java agent runtime smoke failed.' }
} finally {
  Remove-Item -LiteralPath $configPath -Force -ErrorAction SilentlyContinue
}
