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
