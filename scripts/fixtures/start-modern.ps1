param([int]$HttpPort = 18080, [int]$MySqlPort = 3307, [string]$ExtensionId = '', [string]$Revision = 'modern-A', [switch]$DeployRevisionOnly)
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$cache = Join-Path $root '.fixture-cache'
if ($ExtensionId -and $ExtensionId -notmatch '^[a-p]{32}$') { throw 'Extension ID is invalid.' }
$hostRegistryPaths = @('HKCU:\Software\Google\Chrome\NativeMessagingHosts\io.legacylens.host','HKCU:\Software\Chromium\NativeMessagingHosts\io.legacylens.host')
if (!$DeployRevisionOnly -and $ExtensionId) { foreach ($path in $hostRegistryPaths) { if (Test-Path $path) { throw "Native host registry key already exists: $path" } } }
$statePath = Join-Path $cache 'state.json'
if (!$DeployRevisionOnly -and (Test-Path -LiteralPath $statePath)) { throw 'Fixture state exists; use stop.ps1 before another start.' }
if ($DeployRevisionOnly -and !(Test-Path -LiteralPath $statePath)) { throw 'No active fixture state exists for redeploy.' }
New-Item -ItemType Directory -Force -Path $cache | Out-Null
$lock = Get-Content -Raw (Join-Path $root 'fixtures\modern\fixture-lock.json') | ConvertFrom-Json
function Get-PinnedArchive($name, $entry) {
  $archive = Join-Path $cache ($name + '.zip')
  if (!(Test-Path -LiteralPath $archive)) { curl.exe -LfsS -o $archive $entry.url; if ($LASTEXITCODE -ne 0) { throw "Could not download $name" } }
  if (!$entry.sha256 -and !$entry.sha1 -and !$entry.md5) { throw "No pinned checksum for $name" }
  if ($entry.sha256 -and (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $entry.sha256.ToLowerInvariant()) { throw "SHA-256 mismatch for $name" }
  if ($entry.sha1 -and (Get-FileHash -LiteralPath $archive -Algorithm SHA1).Hash.ToLowerInvariant() -ne $entry.sha1.ToLowerInvariant()) { throw "SHA-1 mismatch for $name" }
  if ($entry.md5 -and (Get-FileHash -LiteralPath $archive -Algorithm MD5).Hash.ToLowerInvariant() -ne $entry.md5.ToLowerInvariant()) { throw "MD5 mismatch for $name" }
  $destination = Join-Path $cache $name
  if (!(Test-Path -LiteralPath $destination)) { Expand-Archive -LiteralPath $archive -DestinationPath $destination }
  return $destination
}
$jdkDir = Get-PinnedArchive 'jdk21' $lock.java
$wildDir = Get-PinnedArchive 'wildfly35' $lock.wildfly
$mysqlDir = Get-PinnedArchive 'mysql84' $lock.mysql
$jdk = Join-Path $jdkDir 'jdk-21.0.5+11'
$wildfly = Join-Path $wildDir 'wildfly-35.0.0.Final'
if (!(Test-Path (Join-Path $jdk 'bin\java.exe')) -or !(Test-Path (Join-Path $wildfly 'bin\standalone.bat')) -or !(Test-Path (Join-Path $mysqlDir 'mysql-8.4.0-winx64\bin\mysqld.exe'))) { throw 'Pinned Java, WildFly, or MySQL archive is incomplete.' }
$env:JAVA_HOME = $jdk
if ($DeployRevisionOnly) {
  $state = Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
  if (!$state.wildflyRoot -or !$state.httpPort) { throw 'Fixture state lacks the WildFly deployment identity.' }
  & mvn -q -f (Join-Path $root 'fixtures\modern\pom.xml') clean package "-Dfixture.revision=$Revision"
  if ($LASTEXITCODE -ne 0) { throw 'Modern WAR revision build failed.' }
  $revisionWar = Join-Path $cache ('modern-fixture-' + $Revision + '.war')
  $revisionDeployScript = Join-Path $cache 'deploy-modern-revision.cli'
  Copy-Item (Join-Path $root 'fixtures\modern\target\modern-fixture.war') $revisionWar -Force
  try {
    $cli = Join-Path ([string]$state.wildflyRoot) 'bin\jboss-cli.bat'
    @('deploy "' + $revisionWar + '" --name=modern-fixture.war --force') | Set-Content -LiteralPath $revisionDeployScript -Encoding ascii
    Push-Location $cache
    try { $deployOutput = & $cli --connect --controller=127.0.0.1:9990 --file=deploy-modern-revision.cli 2>&1; $deployExit = $LASTEXITCODE }
    finally { Pop-Location }
    if ($deployExit -ne 0) { throw ('WildFly revision redeploy failed: ' + ($deployOutput -join ' ')) }
    $deploymentContent = & $cli --connect --controller=127.0.0.1:9990 '--command=/deployment=modern-fixture.war:read-attribute(name=content)' 2>&1
    if ($LASTEXITCODE -ne 0) { throw ('WildFly revision content query failed: ' + ($deploymentContent -join ' ')) }
    $expectedContentHash = (Get-FileHash -LiteralPath $revisionWar -Algorithm SHA1).Hash.ToLowerInvariant()
    $actualContentHash = -join ([regex]::Matches(($deploymentContent -join "`n"), '0x([0-9a-fA-F]{2})') | ForEach-Object { $_.Groups[1].Value.ToLowerInvariant() })
    if ($actualContentHash -ne $expectedContentHash) { throw "WildFly revision content hash mismatch; expected=$expectedContentHash actual=$actualContentHash" }
    $url = "http://127.0.0.1:$($state.httpPort)/modern-fixture/orders.xhtml"
    $ready = $false
    for ($i=0; $i -lt 60; $i++) {
      try { $response = Invoke-WebRequest -Uri $url -TimeoutSec 2; if ($response.StatusCode -eq 200 -and $response.Content -match 'orderForm:saveOrder') { $ready = $true; break } } catch {}
      Start-Sleep -Seconds 1
    }
    if (!$ready) { throw 'WildFly revision readiness failed; inspect sanitized logs in .fixture-cache.' }
    [pscustomobject]@{ url=$url; revision=$Revision; status='redeployed' }
  } finally { Remove-Item -LiteralPath $revisionWar,$revisionDeployScript -Force -ErrorAction SilentlyContinue }
  exit 0
}
& mvn -q -f (Join-Path $root 'fixtures\modern\pom.xml') clean package "-Dfixture.revision=$Revision"
if ($LASTEXITCODE -ne 0) { throw 'Modern WAR build failed.' }
& mvn -q -f (Join-Path $root 'java\pom.xml') -pl agent -am '-DskipTests' clean package
if ($LASTEXITCODE -ne 0) { throw 'Java agent build failed.' }
& go -C (Join-Path $root 'core') build -o (Join-Path $cache 'legacylens.exe') ./cmd/legacylens
if ($LASTEXITCODE -ne 0) { throw 'Core build failed.' }
if ($ExtensionId) {
  & go -C (Join-Path $root 'core') build -o (Join-Path $cache 'legacylens-host.exe') ./cmd/legacylens-host
  if ($LASTEXITCODE -ne 0) { throw 'Native host build failed.' }
}
$fixtureId = 'legacylens-fixture-' + [guid]::NewGuid().ToString('N').Substring(0,12)
$runtime = Join-Path $env:TEMP $fixtureId
New-Item -ItemType Directory -Path $runtime | Out-Null
$schema = Join-Path $root 'fixtures\modern\sql\schema.sql'
$mysqlHome = Join-Path $mysqlDir 'mysql-8.4.0-winx64'
$mysqlData = Join-Path $cache ('mysql-data-' + [guid]::NewGuid().ToString('N').Substring(0,12))
$mysqld = Join-Path $mysqlHome 'bin\mysqld.exe'
$mysql = Join-Path $mysqlHome 'bin\mysql.exe'
$mysqladmin = Join-Path $mysqlHome 'bin\mysqladmin.exe'
try {
  $osInfo = Get-CimInstance Win32_OperatingSystem
  $state = @{ fixtureId=$fixtureId; runtime=$runtime; mysqlData=$mysqlData; mysqlPort=$MySqlPort; platform=@{ name=$osInfo.Caption; architecture=$osInfo.OSArchitecture; version=$osInfo.Version } }
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  New-Item -ItemType Directory -Force -Path $mysqlData | Out-Null
  & $mysqld --no-defaults --initialize-insecure "--basedir=$mysqlHome" "--datadir=$mysqlData" --console
  if ($LASTEXITCODE -ne 0) { throw 'MySQL data directory initialization failed.' }
  $mysqlArguments = @('--no-defaults',"--basedir=`"$mysqlHome`"","--datadir=`"$mysqlData`"","--port=$MySqlPort",'--bind-address=127.0.0.1','--console')
  $mysqlProcess = Start-Process -FilePath $mysqld -ArgumentList $mysqlArguments -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $cache 'mysql-out.log') -RedirectStandardError (Join-Path $cache 'mysql-err.log')
  $state.mysqlPid=$mysqlProcess.Id; $state.mysqlPath=$mysqld
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  $mysqlConnection = @('--no-defaults','--protocol=tcp','--host=127.0.0.1',"--port=$MySqlPort",'--user=root')
  $mysqlReady = $false
  for ($i=0; $i -lt 90; $i++) {
    if ($mysqlProcess.HasExited) { throw 'MySQL server exited during startup.' }
    & $mysql @mysqlConnection --execute='SELECT 1' 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { $mysqlReady = $true; break }
    Start-Sleep -Seconds 1
  }
  if (!$mysqlReady) { throw 'MySQL server readiness failed; inspect sanitized logs in .fixture-cache.' }
  $rootPasswordSql = "ALTER USER 'root'@'localhost' IDENTIFIED BY 'fixture-root';"
  & $mysql @mysqlConnection "--execute=$rootPasswordSql"
  if ($LASTEXITCODE -ne 0) { throw 'Could not configure fixture MySQL root account.' }
  $mysqlRootConnection = $mysqlConnection + @('--password=fixture-root')
  $schemaSql = Get-Content -Raw -LiteralPath $schema
  $schemaSql | & $mysql @mysqlRootConnection
  if ($LASTEXITCODE -ne 0) { throw 'Could not initialize fixture MySQL schema.' }
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
  $project = & $corePath project register --root (Join-Path $root 'fixtures\modern') --name 'Modern fixture' | ConvertFrom-Json
  if ($LASTEXITCODE -ne 0 -or !$project.projectId) { throw 'Fixture project registration failed.' }
  $state.projectId=$project.projectId
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  $foreign = Get-CimInstance Win32_Process -Filter "name='legacylens.exe'" | Where-Object ExecutablePath -eq $corePath
  if ($foreign) { throw 'Fixture core executable is already running without this fixture state.' }
  $discoveryPath = Join-Path $cache 'LegacyLens\discovery.json'
  Remove-Item -LiteralPath $discoveryPath -Force -ErrorAction SilentlyContinue
  $core = Start-Process -FilePath (Join-Path $cache 'legacylens.exe') -ArgumentList 'serve' -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $cache 'core-out.log') -RedirectStandardError (Join-Path $cache 'core-err.log')
  $state.corePid=$core.Id; $state.corePath=$corePath
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  for ($i=0; $i -lt 100 -and !(Test-Path $discoveryPath); $i++) { Start-Sleep -Milliseconds 100 }
  if (!(Test-Path $discoveryPath)) { throw 'Core discovery was not created.' }
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
  @("endpoint=http://$($discovery.address)/v1/events", "token=$($discovery.agentToken)", "projectId=$($project.projectId)", 'producerId=modern-fixture', 'revision=fixture-1', 'packages=io.legacylens.fixture') | Set-Content -LiteralPath $configSource -Encoding ascii
  Copy-Item $configSource $agentConfig -Force
  if ((Get-FileHash $configSource -Algorithm SHA256).Hash -ne (Get-FileHash $agentConfig -Algorithm SHA256).Hash) { throw 'Copied agent config hash differs from generated config.' }
  $fixtureDeployment = Join-Path $cache 'modern-fixture.war'
  Copy-Item (Join-Path $root 'fixtures\modern\target\modern-fixture.war') $fixtureDeployment -Force
  $scannerDeployment = Join-Path $wildfly 'standalone\deployments\modern-fixture.war'
  Remove-Item -LiteralPath $scannerDeployment,($scannerDeployment + '.dodeploy'),($scannerDeployment + '.deployed'),($scannerDeployment + '.failed'),($scannerDeployment + '.isdeploying'),($scannerDeployment + '.isundeploying') -Force -ErrorAction SilentlyContinue
  $env:MODERN_DB_PASSWORD = 'fixture-only-password'
  $env:MODERN_DB_URL = "jdbc:mysql://127.0.0.1:$MySqlPort/modern?useSSL=false&allowPublicKeyRetrieval=true"
  $wildflyJava = Join-Path $jdk 'bin\java.exe'
  $wildflyArguments = @('-D[Standalone]','-server','-Xms256m','-Xmx512m','-Djava.net.preferIPv4Stack=true','-Djboss.modules.system.pkgs=org.jboss.byteman','-Djava.awt.headless=true',"-javaagent:`"$agentJar`"=config=`"$agentConfig`"", "-Dorg.jboss.boot.log.file=`"$(Join-Path $wildfly 'standalone\log\server.log')`"", "-Dlogging.configuration=`"file:$(Join-Path $wildfly 'standalone\configuration\logging.properties')`"",'-jar',"`"$(Join-Path $wildfly 'jboss-modules.jar')`"",'-mp',"`"$(Join-Path $wildfly 'modules')`"",'org.jboss.as.standalone',"-Djboss.home.dir=`"$wildfly`"", "-Djboss.server.base.dir=`"$(Join-Path $wildfly 'standalone')`"",'-b','127.0.0.1',"-Djboss.http.port=$HttpPort")
  $wildflyStart = New-Object System.Diagnostics.ProcessStartInfo
  $wildflyStart.FileName = $wildflyJava
  $wildflyStart.Arguments = $wildflyArguments -join ' '
  $wildflyStart.WorkingDirectory = $wildfly
  $wildflyStart.UseShellExecute = $false
  $wildflyStart.CreateNoWindow = $true
  $wildflyStart.RedirectStandardOutput = $true
  $wildflyStart.RedirectStandardError = $true
  $wild = New-Object System.Diagnostics.Process
  $wild.StartInfo = $wildflyStart
  if (!$wild.Start()) { throw 'WildFly JVM could not be started.' }
  $wildflyStdout = $wild.StandardOutput.ReadToEndAsync()
  $wildflyStderr = $wild.StandardError.ReadToEndAsync()
  $state.wildflyPid=$wild.Id; $state.wildflyPath=$wildflyJava; $state.wildflyRoot=$wildfly; $state.projectId=$project.projectId; $state.httpPort=$HttpPort
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  $cli = Join-Path $wildfly 'bin\jboss-cli.bat'
  $managementReady = $false
  for ($i=0; $i -lt 120; $i++) {
    if ($wild.HasExited) {
      $startupOutput = @($wildflyStdout.GetAwaiter().GetResult(),$wildflyStderr.GetAwaiter().GetResult()) -join "`n"
      $startupErrors = @($startupOutput -split "`r?`n" | Where-Object { $_ -match '(?i)error|exception|failed|could not|invalid|usage:' } | Select-Object -Last 8)
      throw "WildFly JVM exited during startup with code $($wild.ExitCode): $($startupErrors -join ' | ')"
    }
    $managementState = & $cli --connect --controller=127.0.0.1:9990 '--command=:read-attribute(name=server-state)' 2>&1
    if ($LASTEXITCODE -eq 0 -and ($managementState -join ' ') -match 'running') { $managementReady = $true; break }
    Start-Sleep -Seconds 1
  }
  if (!$managementReady) { throw 'WildFly management readiness failed; inspect sanitized logs in .fixture-cache.' }
  $productInfo = & $cli --connect --controller=127.0.0.1:9990 '--command=:product-info' 2>&1
  if ($LASTEXITCODE -ne 0) { throw ('WildFly runtime version query failed: ' + ($productInfo -join ' ')) }
  $wildflyVersionMatch = [regex]::Match(($productInfo -join "`n"), 'product-version"?\s*=>\s*"([^"]+)"')
  if (!$wildflyVersionMatch.Success) { throw ('WildFly runtime version was not present in :product-info: ' + ($productInfo -join ' ')) }
  $state.wildflyVersion = $wildflyVersionMatch.Groups[1].Value
  $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
  $cli = Join-Path $wildfly 'bin\jboss-cli.bat'
  @('deploy "' + $fixtureDeployment + '" --name=modern-fixture.war --force') | Set-Content -LiteralPath (Join-Path $cache 'deploy-fixture.cli') -Encoding ascii
  Push-Location $cache
  try { $deploymentOutput = & $cli --connect --controller=127.0.0.1:9990 --file=deploy-fixture.cli 2>&1; $deploymentExit = $LASTEXITCODE }
  finally { Pop-Location }
  if ($deploymentExit -ne 0) { throw ('WildFly fixture redeploy failed: ' + ($deploymentOutput -join ' ')) }
  $deploymentContent = & $cli --connect --controller=127.0.0.1:9990 '--command=/deployment=modern-fixture.war:read-attribute(name=content)' 2>&1
  if ($LASTEXITCODE -ne 0) { throw ('WildFly fixture content query failed: ' + ($deploymentContent -join ' ')) }
  $expectedContentHash = (Get-FileHash -LiteralPath $fixtureDeployment -Algorithm SHA1).Hash.ToLowerInvariant()
  $actualContentHash = -join ([regex]::Matches(($deploymentContent -join "`n"), '0x([0-9a-fA-F]{2})') | ForEach-Object { $_.Groups[1].Value.ToLowerInvariant() })
  if ($actualContentHash -ne $expectedContentHash) { throw "WildFly deployment content hash mismatch; expected=$expectedContentHash actual=$actualContentHash output=$($deploymentContent -join ' ')" }
  $url = "http://127.0.0.1:$HttpPort/modern-fixture/orders.xhtml"
  $ready = $false
  for ($i=0; $i -lt 120; $i++) {
    if ($wild.HasExited) {
      $startupOutput = @($wildflyStdout.GetAwaiter().GetResult(),$wildflyStderr.GetAwaiter().GetResult()) -join "`n"
      $startupErrors = @($startupOutput -split "`r?`n" | Where-Object { $_ -match '(?i)error|exception|failed|could not|invalid|usage:' } | Select-Object -Last 8)
      throw "WildFly JVM exited during startup with code $($wild.ExitCode): $($startupErrors -join ' | ')"
    }
    try { $response = Invoke-WebRequest -Uri $url -TimeoutSec 2; if ($response.StatusCode -eq 200 -and $response.Content -match 'orderForm:saveOrder') { $ready=$true; break } } catch {}
    Start-Sleep -Seconds 1
  }
  if (!$ready) { throw 'WildFly fixture readiness failed; inspect sanitized logs in .fixture-cache.' }
  [pscustomobject]@{ url=$url; projectId=$project.projectId; status='ready' }
} catch {
  & (Join-Path $PSScriptRoot 'stop.ps1')
  throw
}
