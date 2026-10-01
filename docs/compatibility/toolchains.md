# Toolchains

Initial pinned toolchains and contract package dependencies for this implementation:

| Component | Version | Evidence |
| --- | --- | --- |
| Go | 1.26.2, Windows x64 | `go version` on the implementation host |
| Node.js | 22.15.0 | `node --version` on the implementation host |
| npm | 10.9.2 | Installed npm CLI version |
| TypeScript | 5.9.3 | Exact `packages/contracts` development dependency and lockfile |
| Vitest | 5.0.3 | Exact `packages/contracts` development dependency and lockfile |
| JSON Schema | Draft 2020-12 | `$schema` in both versioned contract files |

The repository records the Node.js and npm versions in the root `engines` field. Go's module declares language version 1.26.0; the verified implementation toolchain is Go 1.26.2. Contract package dependencies use exact versions in `packages/contracts/package.json` and `package-lock.json`. No global toolchains were installed or changed.

## Java agent (Task 6)

| Component | Exact version | Runtime / license evidence |
| --- | --- | --- |
| Eclipse Temurin JDK | 8u504-b01, Windows x64 portable ZIP | Official [pinned release](https://github.com/adoptium/temurin8-binaries/releases/tag/jdk8u504-b01); archive SHA-256 `ea43d46ede95b51e44a12c66711706cddc762e0a766c54bccea18954e902b2aa`, verified against the official companion checksum. GPL-2.0 with Classpath Exception. |
| GraalVM JDK | 21.0.8+12.1 | Preinstalled validation runtime at the implementation host; no system environment change. |
| Byte Buddy | 1.18.7 (`byte-buddy`) | Apache-2.0; official [compatibility table](https://github.com/raphw/byte-buddy#java-version-compatibility) lists minimum JVM 8 and support for Java 25+, including Java 21 class files. Its bundled ASM is BSD-3-Clause. |
| JUnit Jupiter | 5.10.2 | EPL-2.0; Maven artifact pinned in `java/agent/pom.xml`; test dependency only. The [official 5.10.2 guide](https://junit.org/junit5/docs/5.10.2/user-guide/) documents its runtime support. |
| Maven Compiler Plugin | 3.13.0 | Apache-2.0; `--release 8` bytecode target. |
| Maven Surefire Plugin | 3.2.5 | Apache-2.0; focused JUnit 5 execution. |
| Maven Shade Plugin | 3.6.1 | Apache-2.0; its [release notes](https://github.com/apache/maven-shade-plugin/releases/tag/maven-shade-plugin-3.6.1) record ASM 9.8 for Java 25 class files; relocates `net.bytebuddy` to `io.legacylens.internal.bytebuddy` in the agent JAR. |

The Task 6 agent runtime is Java 8, while implementation and required cross-version tests run on the pinned Java 21 toolchain above. Transitive dependency/license inventory is deferred to packaging as specified by the implementation plan.

When the bounded active-trace state table is full, the agent rejects a new trace without assigning the loss to an existing trace. The background transport thread writes a coalesced structured line to the agent process stderr, for example `{"kind":"agent.capacity_rejected","count":3}`. It emits at most one line per second, includes no trace IDs or credentials, and preserves rejections that arrive while the diagnostic sink is writing. Capture remains nonblocking on application threads; check the agent process stderr for this capacity signal.
