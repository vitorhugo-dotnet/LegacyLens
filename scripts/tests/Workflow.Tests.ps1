Describe 'Task 18 CI and release gates' {
  BeforeAll {
    $root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
    $ci = Get-Content -Raw (Join-Path $root '.github/workflows/ci.yml')
    $release = Get-Content -Raw (Join-Path $root '.github/workflows/release.yml')
    $packager = Get-Content -Raw (Join-Path $root 'scripts/package-release.ps1')
  }

  It 'runs the modern fixture click and redeploy flows before packaging' {
    if (-not [regex]::IsMatch($ci, 'modern-click\.spec\.ts')) { throw 'Modern click flow is not run by CI.' }
    if (-not [regex]::IsMatch($ci, 'redeploy\.spec\.ts')) { throw 'Modern redeploy flow is not run by CI.' }
    if (-not [regex]::IsMatch($ci, "'modern-fixture'") -or -not [regex]::IsMatch($ci, "'modern-e2e'") -or -not [regex]::IsMatch($ci, "'redeploy'")) { throw 'Modern fixture verification keys are missing from the package workflow.' }
    if (-not [regex]::IsMatch($ci, 'record-package-verification\.ps1') -or -not [regex]::IsMatch($ci, 'packaged-java8-smoke') -or -not [regex]::IsMatch($ci, 'packaged-java21-smoke')) { throw 'Packaged Java 8/21 smoke results are not recorded after execution.' }
  }

  It 'requires every verification job to succeed before publishing' {
    $gate = [regex]::Match($release, "needs\.required-checks\.result == '([^']+)'")
    if (-not $gate.Success -or $gate.Groups[1].Value -ne 'success') { throw 'Publish must require a successful reusable verification workflow.' }
    foreach ($result in @('failure','cancelled','skipped')) { if ($gate.Groups[1].Value -eq $result) { throw "Publish accepts the $result status." } }
    if (-not [regex]::IsMatch($release, 'git rev-list -n 1 \$env:GITHUB_REF')) { throw 'Release does not verify that the tag points to the checked SHA.' }
    if (-not [regex]::IsMatch($release, 'Get-FileHash \$zip -Algorithm SHA256')) { throw 'Release does not verify the package checksum.' }
    if (-not [regex]::IsMatch($release, "'packaged-java8-smoke'") -or -not [regex]::IsMatch($release, "'packaged-java21-smoke'")) { throw 'Release does not require both packaged Java runtime smokes.' }
    if (-not [regex]::IsMatch($ci, 'name: legacylens-windows-x64-\$\{\{ github\.sha \}\}') -or -not [regex]::IsMatch($release, 'legacylens-windows-x64-\$\{\{ github\.sha \}\}')) { throw 'Package artifact is not tied to the same GitHub SHA.' }
  }

  It 'packages the compatibility evidence and closes Task 17 release prerequisites' {
    if (-not [regex]::IsMatch($packager, 'fixtures/modern/release-evidence\.json')) { throw 'Release package omits modern compatibility evidence.' }
    if (-not [regex]::IsMatch($packager, "task17ModernFixture=.*complete.*pending")) { throw 'Task 17 prerequisite is not derived from its E2E checks.' }
    if (-not [regex]::IsMatch($packager, "requiredModernChecks = @\('modern-fixture','modern-e2e','redeploy'\)")) { throw 'Task 17 prerequisite does not require all three modern checks.' }
    if (-not [regex]::IsMatch($packager, "task11JavaWorker=.*complete.*pending") -or -not [regex]::IsMatch($packager, "requiredJavaChecks = @\('java8-runtime','java8-agent-smoke','java8-analyzer-smoke','java21-runtime','java21-agent-smoke','java21-analyzer-smoke','packaged-java8-smoke','packaged-java21-smoke'\)")) { throw 'Task 11 prerequisite is not derived from source and packaged Java 8/21 worker checks.' }
  }
}
