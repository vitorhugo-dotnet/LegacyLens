[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$PackageDirectory,
  [string]$ManifestDirectory = '',
  [string[]]$RegistryRoots = @(
    'HKCU:\Software\Google\Chrome\NativeMessagingHosts',
    'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts'
  )
)
$ErrorActionPreference = 'Stop'
$package = (Resolve-Path -LiteralPath $PackageDirectory).Path
if ([string]::IsNullOrWhiteSpace($ManifestDirectory)) {
  $normalizedPackage = $package.TrimEnd([IO.Path]::DirectorySeparatorChar,[IO.Path]::AltDirectorySeparatorChar).ToLowerInvariant()
  $sha = [Security.Cryptography.SHA256]::Create()
  try { $packageId = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($normalizedPackage))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
  $ManifestDirectory = Join-Path $env:LOCALAPPDATA (Join-Path 'LegacyLens\hosts' $packageId)
}
$manifestDirectoryPath = [IO.Path]::GetFullPath($ManifestDirectory)
$manifestPath = Join-Path $manifestDirectoryPath 'io.legacylens.host.json'
foreach ($registryRoot in $RegistryRoots) {
  $keyPath = Join-Path $registryRoot 'io.legacylens.host'
  if (-not (Test-Path -LiteralPath $keyPath)) { continue }
  $registeredManifest = (Get-Item -LiteralPath $keyPath).GetValue('')
  if ($registeredManifest -and [string]::Equals([IO.Path]::GetFullPath([string]$registeredManifest), $manifestPath, [StringComparison]::OrdinalIgnoreCase)) {
    Remove-Item -LiteralPath $keyPath -Force
  }
}
$stillRegistered = $false
foreach ($registryRoot in $RegistryRoots) {
  $keyPath = Join-Path $registryRoot 'io.legacylens.host'
  if (Test-Path -LiteralPath $keyPath) {
    $registeredManifest = (Get-Item -LiteralPath $keyPath).GetValue('')
    if ($registeredManifest -and [string]::Equals([IO.Path]::GetFullPath([string]$registeredManifest), $manifestPath, [StringComparison]::OrdinalIgnoreCase)) { $stillRegistered = $true }
  }
}
if (-not $stillRegistered -and (Test-Path -LiteralPath $manifestPath)) { Remove-Item -LiteralPath $manifestPath -Force }
Write-Output 'Removed LegacyLens native host registrations owned by this package.'
