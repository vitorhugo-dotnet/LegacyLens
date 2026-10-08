Describe 'Windows release package' {
  BeforeAll {
    $root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
    $schemaPath = Join-Path $root 'packaging/release-manifest.schema.json'
    $evidencePath = Join-Path $root 'fixtures/modern/release-evidence.json'
    $matrix = Get-Content -Raw (Join-Path $root 'docs/compatibility/matrix.json') | ConvertFrom-Json
  }

  It 'includes evidence for both modern interaction capture and revision redeploy' {
    if (-not (Test-Path $evidencePath)) { throw 'Modern fixture release evidence is missing.' }
    $evidence = Get-Content -Raw $evidencePath | ConvertFrom-Json
    $modern = @($evidence.results | Where-Object { $_.id -eq 'jakarta-java21-wildfly35' })
    if ($modern.Count -ne 1 -or $modern[0].status -ne 'verified') { throw 'Verified modern compatibility result is missing.' }
    if ($modern[0].deploymentRevisionScenario.result -notmatch '^verified:') { throw 'Modern redeploy evidence is missing.' }
    if (@($modern[0].tests | Where-Object { $_ -match 'modern-click\.spec\.ts' }).Count -ne 1) { throw 'Modern interaction E2E evidence is missing.' }
    if ($modern[0].stack.wildFly -ne '35.0.0.Final' -or $modern[0].stack.primeFaces -ne '15.0.0') { throw 'Modern runtime evidence does not match the verified matrix.' }
    if ($matrix.results[1].stack.mysql -ne $modern[0].stack.mysql) { throw 'Release evidence differs from the compatibility matrix.' }
  }

  It 'defines a release manifest schema with provenance and verification requirements' {
    if (-not (Test-Path $schemaPath)) { throw 'Release manifest JSON schema is missing.' }
    $schema = Get-Content -Raw $schemaPath | ConvertFrom-Json
    foreach ($field in @('product','version','commit','platform','protocolVersion','componentVersions','verificationResults','releasePrerequisites')) {
      if ($schema.required -notcontains $field) { throw "Manifest schema does not require $field." }
    }
    $manifestPath = Join-Path $root 'artifacts/stage/release-manifest.json'
    if (-not (Test-Path $manifestPath)) { throw 'Packaged release manifest is missing.' }
    $manifest = Get-Content -Raw $manifestPath | ConvertFrom-Json
    foreach ($field in $schema.required) { if (-not $manifest.PSObject.Properties[$field]) { throw "Packaged manifest is missing schema field $field." } }
    if ($manifest.commit -notmatch '^[0-9a-f]{40}$' -or $manifest.platform -ne 'windows/amd64' -or $manifest.protocolVersion -ne 1) { throw 'Packaged manifest provenance does not match the schema.' }
    if (@($manifest.componentVersions.PSObject.Properties | Where-Object { -not $_.Value }).Count -ne 0) { throw 'Packaged manifest has an empty component version.' }
    $checksumPath = Join-Path $root 'artifacts/SHA256SUMS.txt'
    $zipPath = Join-Path $root 'artifacts/legacylens-windows-x64.zip'
    if (-not (Test-Path $checksumPath) -or -not (Test-Path $zipPath)) { throw 'Package ZIP or checksum file is missing.' }
    $checksum = [regex]::Match((Get-Content -Raw $checksumPath).Trim(), '^([0-9a-f]{64})\s+legacylens-windows-x64\.zip$')
    if (-not $checksum.Success -or $checksum.Groups[1].Value -ne (Get-FileHash $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()) { throw 'Package SHA-256 does not match its checksum file.' }
  }

  It 'smoke tests a staged package without Go, Node, or CodeQL on PATH' {
    $package = Join-Path $root 'artifacts/stage'
    if (-not (Test-Path (Join-Path $package 'release-manifest.json'))) { throw 'Staged Windows package is missing.' }
    & (Join-Path $root 'scripts/test-package.ps1') -PackageDirectory $package
    if ($LASTEXITCODE -ne 0) { throw 'Staged package smoke test failed.' }
  }

  It 'records packaged Java 8 and 21 results before marking Task 11 complete' {
    $outputName = 'artifacts/record-package-test-' + [guid]::NewGuid().ToString('N')
    $stage = Join-Path $root ($outputName + '/stage')
    New-Item -ItemType Directory -Path (Join-Path $stage 'fixture') -Force | Out-Null
    try {
      Set-Content -LiteralPath (Join-Path $stage 'fixture/evidence.json') -Value '{}'
      $checks = [ordered]@{
        'java8-runtime'='passed'; 'java8-agent-smoke'='passed'; 'java8-analyzer-smoke'='passed'
        'java21-runtime'='passed'; 'java21-agent-smoke'='passed'; 'java21-analyzer-smoke'='passed'
      }
      $manifest = [ordered]@{
        commit=(git -C $root rev-parse HEAD).Trim()
        verificationResults=$checks
        releasePrerequisites=[ordered]@{ task11JavaWorker='pending'; requiredAssets=@('fixture/evidence.json') }
      }
      $manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $stage 'release-manifest.json') -Encoding utf8
      & (Join-Path $root 'scripts/record-package-verification.ps1') -Check packaged-java8-smoke -OutputDirectory $outputName
      $first = Get-Content -Raw (Join-Path $stage 'release-manifest.json') | ConvertFrom-Json
      if ($first.verificationResults.'packaged-java8-smoke' -ne 'passed' -or $first.releasePrerequisites.task11JavaWorker -ne 'pending') { throw 'Task 11 was marked complete after only the Java 8 package smoke.' }
      & (Join-Path $root 'scripts/record-package-verification.ps1') -Check packaged-java21-smoke -OutputDirectory $outputName
      $final = Get-Content -Raw (Join-Path $stage 'release-manifest.json') | ConvertFrom-Json
      if ($final.verificationResults.'packaged-java21-smoke' -ne 'passed' -or $final.releasePrerequisites.task11JavaWorker -ne 'complete') { throw 'Task 11 was not completed after both Java package smokes.' }
      $sum = [regex]::Match((Get-Content -Raw (Join-Path $root ($outputName + '/SHA256SUMS.txt'))).Trim(), '^([0-9a-f]{64})\s+legacylens-windows-x64\.zip$')
      $zip = Join-Path $root ($outputName + '/legacylens-windows-x64.zip')
      if (-not $sum.Success -or $sum.Groups[1].Value -ne (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant()) { throw 'Verification result archive checksum is not refreshed.' }
    } finally {
      Remove-Item -LiteralPath (Join-Path $root $outputName) -Recurse -Force -ErrorAction SilentlyContinue
    }
  }
}
