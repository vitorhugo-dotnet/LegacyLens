[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][ValidateSet('packaged-java8-smoke','packaged-java21-smoke')][string]$Check,
  [string]$OutputDirectory = 'artifacts'
)
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$out = [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
$stage = Join-Path $out 'stage'
$manifestPath = Join-Path $stage 'release-manifest.json'
$zipPath = Join-Path $out 'legacylens-windows-x64.zip'
if (-not (Test-Path $manifestPath)) { throw 'Staged release manifest is missing.' }
$manifest = Get-Content -Raw $manifestPath | ConvertFrom-Json
if ($manifest.commit -ne (git -C $root rev-parse HEAD).Trim()) { throw 'Package manifest commit differs from the checked source.' }
foreach ($asset in $manifest.releasePrerequisites.requiredAssets) {
  if (-not (Test-Path (Join-Path $stage $asset))) { throw "Required staged asset is missing: $asset" }
}
$manifest.verificationResults | Add-Member -MemberType NoteProperty -Name $Check -Value 'passed' -Force
$requiredJavaChecks = @('java8-runtime','java8-agent-smoke','java8-analyzer-smoke','java21-runtime','java21-agent-smoke','java21-analyzer-smoke','packaged-java8-smoke','packaged-java21-smoke')
$task11Complete = @($requiredJavaChecks | Where-Object { $manifest.verificationResults.$_ -ne 'passed' }).Count -eq 0
$manifest.releasePrerequisites.task11JavaWorker = if ($task11Complete) { 'complete' } else { 'pending' }
$manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding utf8
if (Test-Path $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zipPath
(Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant() + '  legacylens-windows-x64.zip' | Set-Content -LiteralPath (Join-Path $out 'SHA256SUMS.txt') -Encoding ascii
