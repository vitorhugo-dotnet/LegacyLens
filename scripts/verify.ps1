[CmdletBinding()]
param(
  [ValidateSet('packaging','diagnostics')][string]$Scope = 'packaging',
  [string[]]$VerifiedChecks = @()
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $root
try {
  $pester = Get-Command Invoke-Pester -ErrorAction SilentlyContinue
  if (-not $pester) { throw 'Pester is required for the verification scope.' }
  if ($Scope -eq 'diagnostics') {
    $result = Invoke-Pester -Path (Join-Path $PSScriptRoot 'tests/Diagnostics.Tests.ps1') -PassThru
    if ($result.FailedCount -gt 0) { throw "Diagnostics verification failed: $($result.FailedCount) Pester test(s) failed." }
  } else {
    mvn -B -f fixtures/modern/pom.xml clean package '-Dfixture.revision=release'
    & (Join-Path $PSScriptRoot 'package-release.ps1') -OutputDirectory artifacts -VerifiedChecks $VerifiedChecks
    $result = Invoke-Pester -Path (Join-Path $PSScriptRoot 'tests') -PassThru
    if ($result.FailedCount -gt 0) { throw "Packaging verification failed: $($result.FailedCount) Pester test(s) failed." }
  }
} finally { Pop-Location }
