# Task 6 implementation report

## Scope and wire contract

Added a Java 8 `-javaagent` module. Configuration is supplied with `config=<properties-file>` and contains endpoint, bearer token, project ID, producer ID, optional revision, and comma-separated application package allowlist. Credentials do not appear in agent arguments. The default event endpoint is the existing local API `POST /v1/events`; the agent sends protocol version 1, command `trace.ingest`, a request ID, project ID, and one event. Events carry project ID, trace ID, producer ID, positive sequence, event ID, kind, UTC `occurredAt`, optional application revision, and sanitized metadata.

Servlet advice matches both `javax.servlet.Servlet` and `jakarta.servlet.Servlet`, including concrete direct implementations. A servlet request starts capture only when its existing W3C `traceparent` is valid; the trace ID is reused and no synthetic trace is created. Application methods are transformed only under configured package prefixes. Method events carry `code.class`, `code.method`, JVM `code.descriptor`, `code.deployment`, and either `code.line` or explicit `code.line_missing=true`. Deployment identity includes the defining class loader identity. JDBC advice matches MySQL/MariaDB implementations, emits only recognized parameterized SQL, and suppresses nested wrapper/driver duplication. Raw request URI, method arguments, return values, bound SQL values, exception messages, and full source paths are omitted.

Transport uses a bounded 4096-event queue and nonblocking `offer` on instrumented application threads. A daemon sender posts to the API with 250 ms connect/read timeouts. Failed sends and queue overflow are counted per trace; after recovery it submits `agent.loss` for the affected trace and producer. Sequence allocation is concurrent and scoped per trace within this producer, so another trace does not create a false gap.

The Go sanitizer allowlist accepts and validates only `code.class`, `code.method`, `code.descriptor`, `code.deployment`, `code.line`, `code.line_missing`, and `agent.dropped_count` among the new fields. A route-level test sends through the actual `/v1/events` server and CaptureService into sanitization; arbitrary metadata, exception text, and unsafe SQL values remain removed.

## Focused evidence

Go RED before the allowlist change:

```text
go -C core test ./internal/domain -run '^TestSanitizeMetadataKeepsStructuredJavaMethodIdentity$' -count=1
FAIL: safe structured identity "code.class" was lost: map[string]string{}
```

Go GREEN after the change, with Task 5 scoped caches:

```text
$env:GOCACHE="$env:TEMP\legacylens-task5-gocache"
$env:GOMODCACHE="$env:TEMP\legacylens-task5-gomodcache"
go -C core test ./internal/domain ./internal/application ./internal/adapters/localapi -run 'Test(SanitizeMetadata|Ingest|Capture|Trace|EventsEndpointIngestsAndSanitizesStructuredJavaIdentity)' -count=1
ok legacylens/core/internal/domain
ok legacylens/core/internal/application
ok legacylens/core/internal/adapters/localapi
```

Java focused tests use only `AgentFlowTest` and `AgentFailureTest`:

```powershell
$env:JAVA_HOME='F:\graalvm-jdk-21.0.8+12.1'
$env:Path="$env:JAVA_HOME\bin;F:\apache-maven-3.9.11\bin;$env:Path"
& 'F:\apache-maven-3.9.11\bin\mvn.cmd' -f java/pom.xml -pl agent -am '-Dtest=AgentFlowTest,AgentFailureTest' '-Dsurefire.failIfNoSpecifiedTests=false' "-Dmaven.repo.local=$env:TEMP\legacylens-task6-m2" test
```

Result on GraalVM 21.0.8+12.1: 8 tests, 0 failures/errors. The same focused command passed on portable Temurin 8u504-b01: 8 tests, 0 failures/errors. The final Java 8 run compiled all 9 production and 10 test source files with `javac [debug release 8]` after removing only the workspace `java/agent/target` directory. Maven's clean plugin 3.2.0 was not cached and a network attempt was denied, so target was cleared directly after verifying its resolved path remained within the workspace.

`AgentFlowTest` attaches the real Byte Buddy instrumentation, transforms fixture servlet/application/JDBC classes as they load, and observes the `http.server` → application method → single `db.query` events. It checks both servlet namespaces, same-name classes loaded in separate class loaders, line metadata, trace reuse, positive monotonic sequence, UTC timestamps, sanitized SQL, and absence of a secret exception message. This transformation test is separate from the packaged-agent startup smoke test.

`AgentFailureTest` checks queue capacity 4096 and nonblocking overflow, concurrent sequence uniqueness, unsafe SQL rejection, and a local HTTP server that first returns 503 then recovers. It verifies a trace-scoped `agent.loss` diagnostic and no loss report assigned to another trace. A controlled post-implementation sensitivity probe temporarily disabled servlet matching and loss flushing: 3 of 8 tests failed (HTTP/app/JDBC flow, Jakarta servlet capture, and loss recovery). Both changes were restored, and the final Java 8 and Java 21 runs passed. This probe is not reported as pre-implementation Java RED; the earlier Java setup/dependency failures are not represented as feature RED either.

## Artifact and runtime proof

Pinned dependencies are Byte Buddy 1.18.7 (Apache-2.0; official compatibility table lists minimum JVM 8 and Java 21 support), JUnit Jupiter 5.10.2 (EPL-2.0), Maven Compiler Plugin 3.13.0, Surefire 3.2.5, and Shade Plugin 3.6.1. Exact source and license references are recorded in `docs/compatibility/toolchains.md`. Shade 3.5.3 initially failed while reading Byte Buddy's Java 25 multi-release class files (class major 68); Shade 3.6.1's ASM 9.8 handled them. The final shaded jar contains relocated `io/legacylens/internal/bytebuddy/agent/builder/AgentBuilder.class`; `javap -verbose ... LegacyLensAgent` reports major version 52. JDK 8 launched the final jar using `-javaagent` with exit code 0. This proves startup/compatibility, while the preceding tests prove actual transformations.

Packaging and startup proof commands:

```powershell
$env:JAVA_HOME="$env:TEMP\legacylens-task6-jdk8\jdk8u504-b01"
$env:Path="$env:JAVA_HOME\bin;F:\apache-maven-3.9.11\bin;$env:Path"
& 'F:\apache-maven-3.9.11\bin\mvn.cmd' -f java/pom.xml -pl agent -am '-DskipTests' "-Dmaven.repo.local=$env:TEMP\legacylens-task6-m2" package
# BUILD SUCCESS; Shade 3.6.1 included relocated Byte Buddy classes
& "$env:JAVA_HOME\bin\javap.exe" -verbose -classpath java/agent/target/agent-0.1.0.jar io.legacylens.agent.LegacyLensAgent | Select-String 'major version'
# major version: 52
& "$env:JAVA_HOME\bin\java.exe" "-javaagent:java/agent/target/agent-0.1.0.jar=config=$env:TEMP\legacylens-task6-agent.properties" -cp java/agent/target/test-classes io.legacylens.agent.AgentSmokeMain
# exit code 0
```

Portable runtime: Eclipse Temurin 8u504-b01 Windows x64 archive from the official Adoptium release. Its SHA-256 was checked against the official companion checksum: `ea43d46ede95b51e44a12c66711706cddc762e0a766c54bccea18954e902b2aa`. Archive/extraction were kept under `%TEMP%\legacylens-task6-jdk8-download` and `%TEMP%\legacylens-task6-jdk8\jdk8u504-b01`. Maven artifacts were isolated in `%TEMP%\legacylens-task6-m2`; no global Java settings were changed.

Final artifact SHA-256: `8B19AD1F73896AF78D9468985D16CCB736151912062599DF06192A63B8BBA42D` (`java/agent/target/agent-0.1.0.jar`, generated and ignored).

## Limits

Validation used local servlet/JDBC fixtures and the actual local Go event route, not a deployed WildFly/Jakarta EE application or a production database. The agent recognizes the configured package prefixes and MySQL/MariaDB driver packages; other servlet containers, JDBC vendors, asynchronous context propagation, and deployment-specific class-loader behavior remain unverified. No Task 7 work or full test suite was run. The initial Maven clean-plugin download was blocked by network policy; focused tests and packaging completed using the task-scoped Maven cache.

## Review fix round 1 (base `35874ad`)

All six Important findings are addressed. Focused JUnit execution used JDK 8u504 and GraalVM JDK 21.0.8+12.1; each run passed `AgentFailureTest` (5 tests) and `AgentFlowTest` (7 tests), 12 total, no failures/errors. The six review requirements and direct evidence are:

| Review finding | Regression evidence | Result |
|---|---|---|
| HTTP → method → JDBC causal parent IDs, including exceptional unwind | `AgentFlowTest.adviceTransformsLoadedApplicationMethodAndCapturesExceptionWithoutMessage` asserts the exact HTTP→application method→JDBC parent chain, error parent, and JSON `parentEventId` | Pass |
| Helper visibility in a non-parent-delegating loader | `AgentFlowTest.isolatedNonDelegatingLoaderCanResolveAgentBridge` defines the application fixture with `ClassLoader(null)` and executes instrumented advice through the bootstrap bridge | Pass |
| Prepared statement no-arg execution with safe prepared SQL identity | `AgentFlowTest.preparedStatementExecutionCarriesOnlyPreparedSqlIdentity` executes `prepareStatement`→`setString`→no-arg `executeQuery`; asserts one execution event with sanitized SQL and no bound value/preparation event | Pass |
| Source line when present and explicit absence when absent | Flow test asserts exact `code.line=5`; `AgentFlowTest.noDebugLineHasExplicitAbsence` verifies a generated class without `LineNumberTable` yields `code.line_missing=true` after real transformation | Pass |
| Loss racing a successful recovery report | `AgentFailureTest.concurrentQueueLossDuringSuccessfulRecoveryRemainsReportable` blocks at the successful-send hook, adds another loss for the same trace, then requires a second diagnostic | Pass |
| Bounded trace state, settled retirement, active continuity, and reused trace | `AgentFailureTest.traceSequenceStateRetiresWithNewProducerEpochAndCapacityIsGlobal` checks cap rejection, active sequence continuation, settled retirement, trace reuse under a new producer epoch at sequence 1, and global rejection count | Pass |

The bridge is a single JDK-only class appended to bootstrap search; its methods reflect into the system-loaded agent. The smoke entry point now verifies the bridge class loader is bootstrap and that at least four events (HTTP, application method entry/exit, and JDBC) were allocated by the final shaded `-javaagent` run. The isolated-loader transformation assertion is separately in the focused attach-based `AgentFlowTest`; smoke evidence does not claim a deployed application-server test. `AgentTransport` scopes counters and queued/in-flight/loss state to an effective producer ID formed from the configured producer base plus a process-unique nonce/epoch. Settled states retire; a later reuse of the same trace ID has a different producer and starts at sequence 1, so no order is implied across epochs. At the configured state cap new trace acquisition is rejected without blocking application work and counted by a global capacity-rejection diagnostic, never charged to another trace. Prepared SQL identity is held in a synchronized `WeakHashMap`, so statements are not retained solely by the agent after the application releases them.

The actual feature RED before these fixes was the focused JDK 8 test run: 7 tests, 3 failures and 2 errors, including missing causal parent, no prepared execution, and `NoClassDefFoundError` for the helper in the no-parent loader. A later no-debug fixture attempt using in-test `ToolProvider` compilation hit a Windows/Javac locked-jar `AccessDeniedException`; this was a harness/setup issue. The no-debug fixture was changed to Byte Buddy generated bytes and verifies the absence of `LineNumberTable` before instrumentation. The isolated-loader harness now ends contexts in `finally`. The post-implementation sensitivity probe remains labeled separately in the original report; it is not claimed as RED evidence.

Exact focused commands (run once per JDK, with process-local environment overrides):

```powershell
$env:JAVA_HOME="$env:TEMP\legacylens-task6-jdk8-r1\jdk8u504-b01" # JDK 8u504-b01
$env:Path="$env:JAVA_HOME\bin;F:\apache-maven-3.9.11\bin;$env:Path"
& 'F:\apache-maven-3.9.11\bin\mvn.cmd' -f java/pom.xml -pl agent -am '-Dtest=AgentFlowTest,AgentFailureTest' '-Dsurefire.failIfNoSpecifiedTests=false' "-Dmaven.repo.local=$env:TEMP\legacylens-task6-m2" test
# BUILD SUCCESS; AgentFailureTest 5/5, AgentFlowTest 7/7.
$env:JAVA_HOME='F:\graalvm-jdk-21.0.8+12.1' # GraalVM JDK 21.0.8+12.1
$env:Path="$env:JAVA_HOME\bin;F:\apache-maven-3.9.11\bin;$env:Path"
& 'F:\apache-maven-3.9.11\bin\mvn.cmd' -f java/pom.xml -pl agent -am '-Dtest=AgentFlowTest,AgentFailureTest' '-Dsurefire.failIfNoSpecifiedTests=false' "-Dmaven.repo.local=$env:TEMP\legacylens-task6-m2" test
# BUILD SUCCESS; AgentFailureTest 5/5, AgentFlowTest 7/7.
$env:GOCACHE="$env:TEMP\legacylens-task5-go-cache"
go test ./internal/adapters/localapi -run '^TestEventsEndpointIngestsAndSanitizesStructuredJavaIdentity$' -count=1
# ok legacylens/core/internal/adapters/localapi 0.923s
```

The last Go command runs with working directory `core/`; it validates the existing Java identity metadata allowlist and real event ingestion route. The observed first attempt from repository root only reported that the Go module is under `core/` and did not execute tests.

Final artifact proof, after compiling the smoke assertion:

```powershell
$env:JAVA_HOME="$env:TEMP\legacylens-task6-jdk8-r1\jdk8u504-b01"
& 'F:\apache-maven-3.9.11\bin\mvn.cmd' -f java/pom.xml -pl agent -am '-DskipTests' "-Dmaven.repo.local=$env:TEMP\legacylens-task6-m2" package
# BUILD SUCCESS; Shade 3.6.1 relocates Byte Buddy under io/legacylens/internal/bytebuddy.
& "$env:JAVA_HOME\bin\java.exe" "-javaagent:java/agent/target/agent-0.1.0.jar=config=$env:TEMP\legacylens-task6-agent.properties" -cp java/agent/target/test-classes io.legacylens.agent.AgentSmokeMain
# exit 0; the main asserts bootstrap AgentBridge visibility and event sequence >= 4.
& "$env:JAVA_HOME\bin\javap.exe" -verbose -classpath java/agent/target/agent-0.1.0.jar io.legacylens.agent.LegacyLensAgent | Select-String 'major version'
# major version: 52
```

The final smoke uses the intentionally offline local endpoint, so events remain in agent-local loss state after the assertion; this verifies transformed event creation and bridge resolution, not successful HTTP delivery. Delivery and Go sanitization are independently verified by the focused Go route test. Final shaded JAR SHA-256: `F0CB12DF72239851F5DD53C48AFAED30374649514B223D2D3FA58E546CE73B37`.
