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
New-Item -ItemType Directory -Force -Path $stage | Out-Null
Push-Location $root
try {
  npm run build --workspace apps/extension
  go -C core build -trimpath -ldflags='-s -w' -o (Join-Path $stage 'legacylens.exe') ./cmd/legacylens
  go -C core build -trimpath -ldflags='-s -w' -o (Join-Path $stage 'legacylens-host.exe') ./cmd/legacylens-host
  mvn -B -f java/pom.xml -pl agent -am package -DskipTests
  New-Item -ItemType Directory -Force -Path (Join-Path $stage 'extension'),(Join-Path $stage 'agent'),(Join-Path $stage 'docs') | Out-Null
  Copy-Item apps/extension/.output/chrome-mv3/* (Join-Path $stage 'extension') -Recurse -Force
  Copy-Item java/agent/target/agent-0.1.0.jar (Join-Path $stage 'agent/legacylens-agent.jar')
  Copy-Item README.md (Join-Path $stage 'README.md')
  Copy-Item LICENSE (Join-Path $stage 'LICENSE')
  $sha = (git rev-parse HEAD).Trim()
  $extension = Get-Content apps/extension/package.json -Raw | ConvertFrom-Json
  $agent = Get-Content java/agent/pom.xml -Raw
  if ($agent -notmatch '<artifactId>agent</artifactId>\s*<properties>') { throw 'Could not read the agent module metadata.' }
  $agentVersion = [regex]::Match($agent, '<version>([^<]+)</version>').Groups[1].Value
  $goVersion = (go version).Trim()
  $nodeVersion = (node --version).Trim()
  $javaVersion = ((java -version 2>&1 | Select-Object -First 1) -replace '.*version "([^"]+)".*','$1')
  if (-not $env:GITHUB_REF_NAME) { $releaseVersion = '0.0.0-dev' } else { $releaseVersion = $env:GITHUB_REF_NAME }
  $results = [ordered]@{}
  foreach ($check in $VerifiedChecks) { $results[$check] = 'passed' }
  $manifest = [ordered]@{
    product='LegacyLens'; version=$releaseVersion; commit=$sha; platform='windows/amd64'; protocolVersion=1
    componentVersions=[ordered]@{ core=$sha; nativeHost=$sha; extension=$extension.version; javaAgent=$agentVersion; goToolchain=$goVersion; nodeToolchain=$nodeVersion; javaRuntime=$javaVersion }
    components=@('core CLI','native messaging host','unpacked Chromium extension','Java 8-compatible instrumentation agent')
    verificationResults=$results
    releasePrerequisites=[ordered]@{ task11JavaWorker='pending'; task17ModernFixture='pending'; requiredAssets=@('analyzer/legacylens-analyzer.jar','fixtures/modern/release-evidence.json') }
    exclusions=@('Java analysis worker not implemented','modern fixture and redeploy validation pending','private Windows 10 system not validated')
  }
  $manifest | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $stage 'release-manifest.json') -Encoding utf8
  & "$PSScriptRoot/test-package.ps1" -PackageDirectory $stage
  $manifest.verificationResults['packaged-core-smoke'] = 'passed'
  $manifest | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $stage 'release-manifest.json') -Encoding utf8
  $zip = Join-Path $out 'legacylens-windows-x64.zip'
  if (Test-Path $zip) { Remove-Item $zip -Force }
  Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
  (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant() + '  legacylens-windows-x64.zip' | Set-Content (Join-Path $out 'SHA256SUMS.txt') -Encoding ascii
} finally { Pop-Location }
