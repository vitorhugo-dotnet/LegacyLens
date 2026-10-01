param([int]$HttpPort = 18080, [int]$MySqlPort = 3307, [string]$ExtensionId = '')
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$cache = Join-Path $root '.fixture-cache'
if ($ExtensionId -and $ExtensionId -notmatch '^[a-p]{32}$') { throw 'Extension ID is invalid.' }
$hostRegistryPaths = @('HKCU:\Software\Google\Chrome\NativeMessagingHosts\io.legacylens.host','HKCU:\Software\Chromium\NativeMessagingHosts\io.legacylens.host')
if ($ExtensionId) { foreach ($path in $hostRegistryPaths) { if (Test-Path $path) { throw "Native host registry key already exists: $path" } } }
$statePath = Join-Path $cache 'state.json'
if (Test-Path -LiteralPath $statePath) { throw 'Fixture state exists; use stop.ps1 before another start.' }
New-Item -ItemType Directory -Force -Path $cache | Out-Null
$lock = Get-Content -Raw (Join-Path $root 'fixtures\legacy\fixture-lock.json') | ConvertFrom-Json
function Get-PinnedArchive($name, $entry) {
  $archive = Join-Path $cache ($name + '.zip')
  if (!(Test-Path -LiteralPath $archive)) { curl.exe -LfsS -o $archive $entry.url; if ($LASTEXITCODE -ne 0) { throw "Could not download $name" } }
  if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.sha256) { throw "Checksum mismatch for $name" }
  $destination = Join-Path $cache $name
  if (!(Test-Path -LiteralPath $destination)) { Expand-Archive -LiteralPath $archive -DestinationPath $destination }
  return $destination
}
$jdkDir = Get-PinnedArchive 'jdk8' $lock.java
$wildDir = Get-PinnedArchive 'wildfly10' $lock.wildfly
$jdk = Join-Path $jdkDir 'jdk8u462-b08'
$wildfly = Join-Path $wildDir 'wildfly-10.0.0.Final'
if (!(Test-Path (Join-Path $jdk 'bin\java.exe')) -or !(Test-Path (Join-Path $wildfly 'bin\standalone.bat'))) { throw 'Pinned Java or WildFly archive is incomplete.' }
$env:JAVA_HOME = $jdk
& mvn -q -f (Join-Path $root 'fixtures\legacy\pom.xml') package
if ($LASTEXITCODE -ne 0) { throw 'Legacy WAR build failed.' }
& mvn -q -f (Join-Path $root 'java\pom.xml') -pl agent -am '-DskipTests' package
if ($LASTEXITCODE -ne 0) { throw 'Java agent build failed.' }
& go -C (Join-Path $root 'core') build -o (Join-Path $cache 'legacylens.exe') ./cmd/legacylens
if ($LASTEXITCODE -ne 0) { throw 'Core build failed.' }
if ($ExtensionId) {
  & go -C (Join-Path $root 'core') build -o (Join-Path $cache 'legacylens-host.exe') ./cmd/legacylens-host
  if ($LASTEXITCODE -ne 0) { throw 'Native host build failed.' }
}
$containerName = 'legacylens-fixture-' + [guid]::NewGuid().ToString('N').Substring(0,12)
$runtime = Join-Path $env:TEMP $containerName
New-Item -ItemType Directory -Path $runtime | Out-Null
$schema = Join-Path $root 'fixtures\legacy\sql\schema.sql'
try {
  $containerId = & docker run --name $containerName --rm -d -p "127.0.0.1:${MySqlPort}:3306" -e 'MYSQL_ROOT_PASSWORD=fixture-root' -v "${schema}:/docker-entrypoint-initdb.d/01-schema.sql:ro" $lock.mysql.image
  if ($LASTEXITCODE -ne 0 -or $containerId -notmatch '^[0-9a-f]{64}$') { throw 'MySQL container start failed.' }
  $state = @{ containerName=$containerName; containerId=$containerId; runtime=$runtime }
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  if ($ExtensionId) {
    $manifestPath = Join-Path $cache 'io.legacylens.host.json'
    @{ name='io.legacylens.host'; description='LegacyLens controlled fixture host'; path=(Join-Path $cache 'legacylens-host.exe'); type='stdio'; allowed_origins=@("chrome-extension://$ExtensionId/") } | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding utf8
    $state.manifestPath=$manifestPath; $state.registryPaths=@()
    foreach ($path in $hostRegistryPaths) {
      New-Item -Path $path -Force | Out-Null
      Set-Item -Path $path -Value $manifestPath
      $state.registryPaths += $path
      $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
    }
  }
  $env:APPDATA = $cache
  if ($ExtensionId) { $env:LEGACYLENS_EXTENSION_ID = $ExtensionId }
  $corePath = Join-Path $cache 'legacylens.exe'
  $foreign = Get-CimInstance Win32_Process -Filter "name='legacylens.exe'" | Where-Object ExecutablePath -eq $corePath
  if ($foreign) { throw 'Fixture core executable is already running without this fixture state.' }
  $discoveryPath = Join-Path $cache 'LegacyLens\discovery.json'
  Remove-Item -LiteralPath $discoveryPath -Force -ErrorAction SilentlyContinue
  $core = Start-Process -FilePath (Join-Path $cache 'legacylens.exe') -ArgumentList 'serve' -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $cache 'core-out.log') -RedirectStandardError (Join-Path $cache 'core-err.log')
  $state.corePid=$core.Id; $state.corePath=$corePath
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  for ($i=0; $i -lt 100 -and !(Test-Path $discoveryPath); $i++) { Start-Sleep -Milliseconds 100 }
  if (!(Test-Path $discoveryPath)) { throw 'Core discovery was not created.' }
  $project = & (Join-Path $cache 'legacylens.exe') project register --root (Join-Path $root 'fixtures\legacy') --name 'Legacy fixture' | ConvertFrom-Json
  if ($LASTEXITCODE -ne 0 -or !$project.projectId) { throw 'Fixture project registration failed.' }
  $discovery = Get-Content -Raw $discoveryPath | ConvertFrom-Json
  $apiPort = [int]($discovery.address -split ':')[-1]
  $listener = Get-NetTCPConnection -OwningProcess $core.Id -LocalPort $apiPort -State Listen -ErrorAction SilentlyContinue
  if (!$listener) { throw 'Discovery address does not belong to the fixture core process.' }
  $agentJar = Join-Path $runtime 'agent.jar'
  $agentSource = Join-Path $root 'java\agent\target\agent-0.1.0.jar'
  Copy-Item $agentSource $agentJar -Force
  if ((Get-FileHash $agentSource -Algorithm SHA256).Hash -ne (Get-FileHash $agentJar -Algorithm SHA256).Hash) { throw 'Copied agent JAR hash differs from built JAR.' }
  $agentConfig = Join-Path $runtime 'agent.properties'
  $configSource = Join-Path $cache 'agent.properties'
  @("endpoint=http://$($discovery.address)/v1/events", "token=$($discovery.agentToken)", "projectId=$($project.projectId)", 'producerId=legacy-fixture', 'revision=fixture-1', 'packages=io.legacylens.fixture') | Set-Content -LiteralPath $configSource -Encoding ascii
  Copy-Item $configSource $agentConfig -Force
  if ((Get-FileHash $configSource -Algorithm SHA256).Hash -ne (Get-FileHash $agentConfig -Algorithm SHA256).Hash) { throw 'Copied agent config hash differs from generated config.' }
  Copy-Item (Join-Path $root 'fixtures\legacy\target\legacy-fixture.war') (Join-Path $wildfly 'standalone\deployments\legacy-fixture.war') -Force
  $env:LEGACY_DB_PASSWORD = 'fixture-only-password'
  $env:LEGACY_DB_URL = "jdbc:mysql://127.0.0.1:$MySqlPort/legacy?useSSL=false"
  $env:JAVA_OPTS = '-Xms256m -Xmx512m -javaagent:"' + $agentJar + '"=config="' + $agentConfig + '"'
  $wild = Start-Process -FilePath (Join-Path $wildfly 'bin\standalone.bat') -ArgumentList '-b','127.0.0.1',"-Djboss.http.port=$HttpPort" -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $cache 'wildfly-out.log') -RedirectStandardError (Join-Path $cache 'wildfly-err.log')
  $state.wildflyPid=$wild.Id; $state.wildflyPath=(Join-Path $wildfly 'bin\standalone.bat'); $state.projectId=$project.projectId; $state.httpPort=$HttpPort
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  $url = "http://127.0.0.1:$HttpPort/legacy-fixture/orders.xhtml"
  $ready = $false
  for ($i=0; $i -lt 120; $i++) { try { $response = Invoke-WebRequest -Uri $url -TimeoutSec 2; if ($response.StatusCode -eq 200 -and $response.Content -match 'orderForm:saveOrder') { $ready=$true; break } } catch {}; Start-Sleep -Seconds 1 }
  if (!$ready) { throw 'WildFly fixture readiness failed; inspect sanitized logs in .fixture-cache.' }
  [pscustomobject]@{ url=$url; projectId=$project.projectId; status='ready' }
} catch {
  & (Join-Path $PSScriptRoot 'stop.ps1')
  throw
}
