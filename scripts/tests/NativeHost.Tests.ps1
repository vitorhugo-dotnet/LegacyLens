Describe 'Native host installation scripts' {
  BeforeAll {
    $root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
    $testId = [guid]::NewGuid().ToString('N')
    $script:registryRoot = "HKCU:\Software\LegacyLens\Pester\$testId"
    $script:installDirectory = Join-Path ([IO.Path]::GetTempPath()) "LegacyLens install $testId"
    New-Item -ItemType Directory -Path $script:installDirectory -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $script:installDirectory 'legacylens-host.exe') -Value 'fixture host'
    $script:otherKey = Join-Path $script:registryRoot 'unrelated.host'
    New-Item -Path $script:otherKey -Force | Out-Null
    $unrelatedValue = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($script:otherKey.Substring('HKCU:\'.Length))
    try { $unrelatedValue.SetValue('', 'C:\other\host.json', [Microsoft.Win32.RegistryValueKind]::String) } finally { $unrelatedValue.Dispose() }
  }

  AfterAll {
    Remove-Item -LiteralPath $script:registryRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $script:installDirectory -Recurse -Force -ErrorAction SilentlyContinue
  }

  It 'installs idempotently under a path with spaces and removes only its own host key' {
    $install = Join-Path $root 'scripts/install-native-host.ps1'
    $uninstall = Join-Path $root 'scripts/uninstall-native-host.ps1'
    & $install -PackageDirectory $script:installDirectory -ManifestDirectory $script:installDirectory -ExtensionId ('a' * 32) -RegistryRoots @($script:registryRoot)
    & $install -PackageDirectory $script:installDirectory -ManifestDirectory $script:installDirectory -ExtensionId ('a' * 32) -RegistryRoots @($script:registryRoot)
    $manifestPath = Join-Path $script:installDirectory 'io.legacylens.host.json'
    if (-not (Test-Path $manifestPath)) { throw 'Native host manifest was not created.' }
    $manifest = Get-Content -Raw $manifestPath | ConvertFrom-Json
    if ($manifest.path -ne (Join-Path $script:installDirectory 'legacylens-host.exe')) { throw 'Host manifest does not preserve the path with spaces.' }
    $registered = (Get-Item -LiteralPath (Join-Path $script:registryRoot 'io.legacylens.host')).GetValue('')
    if ($registered -ne $manifestPath) { throw "Registry default value mismatch; actual=[$registered] expected=[$manifestPath]" }
    & $uninstall -PackageDirectory $script:installDirectory -ManifestDirectory $script:installDirectory -RegistryRoots @($script:registryRoot)
    if (Test-Path (Join-Path $script:registryRoot 'io.legacylens.host')) { throw 'Uninstaller left its native host registry key behind.' }
    if (-not (Test-Path $script:otherKey)) { throw 'Uninstaller removed an unrelated native host registration.' }
    if (Test-Path $manifestPath) { throw 'Uninstaller left its owned manifest behind.' }
  }

  It 'keeps a newer install registered when uninstalling an older package directory' {
    $originalLocalAppData = $env:LOCALAPPDATA
    $testLocalAppData = Join-Path ([IO.Path]::GetTempPath()) ('LegacyLens profile ' + [guid]::NewGuid().ToString('N'))
    $packageA = Join-Path ([IO.Path]::GetTempPath()) ('LegacyLens v1 ' + [guid]::NewGuid().ToString('N'))
    $packageB = Join-Path ([IO.Path]::GetTempPath()) ('LegacyLens v2 ' + [guid]::NewGuid().ToString('N'))
    $registryRoot = "HKCU:\Software\LegacyLens\Pester\upgrade-$([guid]::NewGuid().ToString('N'))"
    New-Item -ItemType Directory -Path $packageA,$packageB -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $packageA 'legacylens-host.exe') -Value 'v1'
    Set-Content -LiteralPath (Join-Path $packageB 'legacylens-host.exe') -Value 'v2'
    try {
      $env:LOCALAPPDATA = $testLocalAppData
      & (Join-Path $root 'scripts/install-native-host.ps1') -PackageDirectory $packageA -ExtensionId ('a' * 32) -RegistryRoots @($registryRoot)
      $keyPath = Join-Path $registryRoot 'io.legacylens.host'
      $manifestA = (Get-Item -LiteralPath $keyPath).GetValue('')
      & (Join-Path $root 'scripts/install-native-host.ps1') -PackageDirectory $packageB -ExtensionId ('a' * 32) -RegistryRoots @($registryRoot)
      $manifestB = (Get-Item -LiteralPath $keyPath).GetValue('')
      if ($manifestA -eq $manifestB) { throw 'Separate package directories unexpectedly share a native host manifest.' }
      & (Join-Path $root 'scripts/uninstall-native-host.ps1') -PackageDirectory $packageA -RegistryRoots @($registryRoot)
      if ((Get-Item -LiteralPath $keyPath).GetValue('') -ne $manifestB) { throw 'Uninstalling the older package removed the newer package registration.' }
      if (-not (Test-Path $manifestB)) { throw 'Uninstalling the older package removed the newer package manifest.' }
      & (Join-Path $root 'scripts/uninstall-native-host.ps1') -PackageDirectory $packageB -RegistryRoots @($registryRoot)
      if (Test-Path $keyPath) { throw 'Uninstalling the current package left its registration behind.' }
    } finally {
      Remove-Item -LiteralPath $registryRoot -Recurse -Force -ErrorAction SilentlyContinue
      Remove-Item -LiteralPath $testLocalAppData,$packageA,$packageB -Recurse -Force -ErrorAction SilentlyContinue
      if ($null -eq $originalLocalAppData) { Remove-Item Env:LOCALAPPDATA -ErrorAction SilentlyContinue } else { $env:LOCALAPPDATA = $originalLocalAppData }
    }
  }
}
