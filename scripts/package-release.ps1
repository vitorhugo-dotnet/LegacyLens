[CmdletBinding()]
param([string]$OutputDirectory = 'artifacts')
$ErrorActionPreference = 'Stop'
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
  $manifest = [ordered]@{ product='LegacyLens'; version=$env:GITHUB_REF_NAME; commit=$sha; platform='windows/amd64'; components=@('core CLI','native messaging host','unpacked Chromium extension','Java 8-compatible instrumentation agent'); exclusions=@('Java analysis worker not implemented','modern fixture and redeploy validation pending','private Windows 10 system not validated') }
  $manifest | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $stage 'release-manifest.json') -Encoding utf8
  $zip = Join-Path $out 'legacylens-windows-x64.zip'
  if (Test-Path $zip) { Remove-Item $zip -Force }
  Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
  (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant() + '  legacylens-windows-x64.zip' | Set-Content (Join-Path $out 'SHA256SUMS.txt') -Encoding ascii
} finally { Pop-Location }
