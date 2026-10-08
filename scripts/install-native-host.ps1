[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$PackageDirectory,
  [Parameter(Mandatory=$true)][string]$ExtensionId,
  [string]$ManifestDirectory = '',
  [string[]]$RegistryRoots = @(
    'HKCU:\Software\Google\Chrome\NativeMessagingHosts',
    'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts'
  )
)
$ErrorActionPreference = 'Stop'
$package = (Resolve-Path -LiteralPath $PackageDirectory).Path
if ($ExtensionId -notmatch '^[a-p]{32}$') { throw 'Extension ID must contain 32 characters in the range a-p.' }
$hostPath = Join-Path $package 'legacylens-host.exe'
if (-not (Test-Path -LiteralPath $hostPath -PathType Leaf)) { throw 'Packaged native host executable is missing.' }
$hostPath = (Resolve-Path -LiteralPath $hostPath).Path
if ([string]::IsNullOrWhiteSpace($ManifestDirectory)) {
  $normalizedPackage = $package.TrimEnd([IO.Path]::DirectorySeparatorChar,[IO.Path]::AltDirectorySeparatorChar).ToLowerInvariant()
  $sha = [Security.Cryptography.SHA256]::Create()
  try { $packageId = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($normalizedPackage))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
  $ManifestDirectory = Join-Path $env:LOCALAPPDATA (Join-Path 'LegacyLens\hosts' $packageId)
}
$manifestDirectoryPath = [IO.Path]::GetFullPath($ManifestDirectory)
$manifestPath = Join-Path $manifestDirectoryPath 'io.legacylens.host.json'
$legacyManifestRoot = [IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'LegacyLens\hosts')).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
$templatePath = Join-Path $PSScriptRoot '..\packaging\native-host.template.json'
$registrations = @()
foreach ($registryRoot in $RegistryRoots) {
  if ($registryRoot -notmatch '^HKCU:\\') { throw 'Native host registrations are restricted to HKCU.' }
  $keyPath = Join-Path $registryRoot 'io.legacylens.host'
  if (Test-Path -LiteralPath $keyPath) {
    $existing = (Get-Item -LiteralPath $keyPath).GetValue('')
    if ($existing -and $existing -ne $manifestPath) {
      $existingIsLegacyLens = $false
      if ((Test-Path -LiteralPath $existing -PathType Leaf) -and [IO.Path]::GetFullPath([string]$existing).StartsWith($legacyManifestRoot, [StringComparison]::OrdinalIgnoreCase)) {
        try {
          $previousManifest = Get-Content -Raw -LiteralPath $existing | ConvertFrom-Json
          $existingIsLegacyLens = ($previousManifest.name -eq 'io.legacylens.host' -and [IO.Path]::GetFileName([string]$previousManifest.path) -eq 'legacylens-host.exe')
        } catch { $existingIsLegacyLens = $false }
      }
      if (-not $existingIsLegacyLens) { throw "Native host registration already belongs to another manifest: $keyPath" }
    }
  }
  $registrations += $keyPath
}
New-Item -ItemType Directory -Path $manifestDirectoryPath -Force | Out-Null
$manifestText = Get-Content -Raw -LiteralPath $templatePath
$manifestText = $manifestText.Replace('${HOST_PATH}', $hostPath.Replace('\','\\')).Replace('${EXTENSION_ID}', $ExtensionId)
$manifest = $manifestText | ConvertFrom-Json
if ($manifest.name -ne 'io.legacylens.host' -or $manifest.path -ne $hostPath -or $manifest.allowed_origins[0] -ne "chrome-extension://$ExtensionId/") { throw 'Native host template expansion failed validation.' }
[IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 4), [Text.UTF8Encoding]::new($false))
foreach ($keyPath in $registrations) {
  New-Item -Path $keyPath -Force | Out-Null
  $relativeKeyPath = $keyPath.Substring('HKCU:\'.Length)
  $registryKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($relativeKeyPath)
  try {
    $registryKey.SetValue('', $manifestPath, [Microsoft.Win32.RegistryValueKind]::String)
    $registryKey.Flush()
  } finally { $registryKey.Dispose() }
}
Write-Output "Installed LegacyLens native host for extension $ExtensionId."
