[CmdletBinding()]
param(
  [string]$OutputDirectory = 'artifacts',
  [string[]]$VerifiedChecks = @()
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$out = [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
$stage = Join-Path $out 'stage'
if ((Test-Path $stage) -and -not $stage.StartsWith($out + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Package staging directory escapes the selected output directory.' }
if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null
$allowedChecks = @('contracts','extension','core','java8-runtime','java8-agent-smoke','java8-analyzer-smoke','java21-runtime','java21-agent-smoke','java21-analyzer-smoke','legacy-fixture','legacy-e2e','modern-fixture','modern-e2e','redeploy')
foreach ($check in $VerifiedChecks) { if ($check -notin $allowedChecks) { throw "Unknown verification result: $check" } }
Push-Location $root
try {
  npm run build --workspace apps/extension
  go -C core build -trimpath -ldflags='-s -w' -o (Join-Path $stage 'legacylens.exe') ./cmd/legacylens
  go -C core build -trimpath -ldflags='-s -w' -o (Join-Path $stage 'legacylens-host.exe') ./cmd/legacylens-host
  mvn -B -f java/pom.xml -pl agent,analyzer -am package -DskipTests
  New-Item -ItemType Directory -Force -Path (Join-Path $stage 'extension'),(Join-Path $stage 'agent'),(Join-Path $stage 'analyzer'),(Join-Path $stage 'docs'),(Join-Path $stage 'fixtures/modern'),(Join-Path $stage 'scripts'),(Join-Path $stage 'packaging') | Out-Null
  Copy-Item apps/extension/.output/chrome-mv3/* (Join-Path $stage 'extension') -Recurse -Force
  Copy-Item java/agent/target/agent-0.1.0.jar (Join-Path $stage 'agent/legacylens-agent.jar')
  Copy-Item java/analyzer/target/analyzer-0.1.0.jar (Join-Path $stage 'analyzer/legacylens-analyzer.jar')
  Copy-Item fixtures/modern/target/modern-fixture.war (Join-Path $stage 'fixtures/modern/modern-fixture.war')
  Copy-Item fixtures/modern/fixture-lock.json,fixtures/modern/release-evidence.json,fixtures/modern/sql/schema.sql (Join-Path $stage 'fixtures/modern')
  Copy-Item scripts/install-native-host.ps1,scripts/uninstall-native-host.ps1 (Join-Path $stage 'scripts')
  Copy-Item packaging/native-host.template.json (Join-Path $stage 'packaging')
  Copy-Item packaging/release-manifest.schema.json (Join-Path $stage 'docs')
  Copy-Item docs/compatibility/matrix.json (Join-Path $stage 'docs/compatibility-matrix.json')
  Copy-Item docs/user/windows-installation.md (Join-Path $stage 'docs/windows-installation.md')
  Copy-Item README.md (Join-Path $stage 'README.md')
  Copy-Item LICENSE (Join-Path $stage 'LICENSE')
  $sha = (git rev-parse HEAD).Trim()
  $extension = Get-Content apps/extension/package.json -Raw | ConvertFrom-Json
  $agent = Get-Content java/agent/pom.xml -Raw
  if ($agent -notmatch '<artifactId>agent</artifactId>\s*<properties>') { throw 'Could not read the agent module metadata.' }
  $agentVersion = [regex]::Match($agent, '<version>([^<]+)</version>').Groups[1].Value
  $analyzer = Get-Content java/analyzer/pom.xml -Raw
  if ($analyzer -notmatch '<artifactId>analyzer</artifactId>') { throw 'Could not read the analyzer module metadata.' }
  $analyzerVersion = [regex]::Match($analyzer, '<parent>\s*<groupId>[^<]+</groupId>\s*<artifactId>[^<]+</artifactId>\s*<version>([^<]+)</version>').Groups[1].Value
  if (-not $analyzerVersion) { throw 'Could not read the analyzer component version.' }
  $goVersion = (go version).Trim()
  $nodeVersion = (node --version).Trim()
  $javaVersion = ((java -version 2>&1 | Select-Object -First 1) -replace '.*version "([^"]+)".*','$1')
  if (-not $env:GITHUB_REF_NAME) { $releaseVersion = '0.0.0-dev' } else { $releaseVersion = $env:GITHUB_REF_NAME }
  $results = [ordered]@{}
  foreach ($check in $VerifiedChecks) { $results[$check] = 'passed' }
  $requiredJavaChecks = @('java8-runtime','java8-agent-smoke','java8-analyzer-smoke','java21-runtime','java21-agent-smoke','java21-analyzer-smoke','packaged-java8-smoke','packaged-java21-smoke')
  $task11Complete = @($requiredJavaChecks | Where-Object { $results[$_] -ne 'passed' }).Count -eq 0
  $requiredModernChecks = @('modern-fixture','modern-e2e','redeploy')
  $task17Complete = @($requiredModernChecks | Where-Object { $results[$_] -ne 'passed' }).Count -eq 0
  $manifest = [ordered]@{
    product='LegacyLens'; version=$releaseVersion; commit=$sha; platform='windows/amd64'; protocolVersion=1
    componentVersions=[ordered]@{ core=$sha; nativeHost=$sha; extension=$extension.version; javaAgent=$agentVersion; javaAnalyzer=$analyzerVersion; goToolchain=$goVersion; nodeToolchain=$nodeVersion; javaRuntime=$javaVersion }
    components=@('core CLI','native messaging host','unpacked Chromium extension','Java 8-compatible instrumentation agent','Java static analyzer worker','verified Jakarta fixture WAR and runtime evidence')
    verificationResults=$results
    releasePrerequisites=[ordered]@{ task11JavaWorker=$(if ($task11Complete) {'complete'} else {'pending'}); task17ModernFixture=$(if ($task17Complete) {'complete'} else {'pending'}); requiredAssets=@('analyzer/legacylens-analyzer.jar','fixtures/modern/modern-fixture.war','fixtures/modern/fixture-lock.json','fixtures/modern/release-evidence.json','fixtures/modern/schema.sql') }
    exclusions=@('Windows 10 x64 validation remains pending on a separate machine','CodeQL CLI is optional and not bundled')
  }
  $manifest | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $stage 'release-manifest.json') -Encoding utf8
  & "$PSScriptRoot/test-package.ps1" -PackageDirectory $stage
  $manifest.verificationResults['packaged-core-smoke'] = 'passed'
  $manifest.verificationResults['packaged-agent-smoke'] = 'passed'
  $manifest.verificationResults['packaged-analyzer-smoke'] = 'passed'
  $manifest | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $stage 'release-manifest.json') -Encoding utf8
  $zip = Join-Path $out 'legacylens-windows-x64.zip'
  if (Test-Path $zip) { Remove-Item $zip -Force }
  Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
  (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant() + '  legacylens-windows-x64.zip' | Set-Content (Join-Path $out 'SHA256SUMS.txt') -Encoding ascii
} finally { Pop-Location }
