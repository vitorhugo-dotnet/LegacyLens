# LegacyLens CodeQL query pack

This pack contains informational Java queries used by LegacyLens to enrich source
analysis with direct method calls and Servlet request-handler methods. The query
results are tables for the adapter; they are not SARIF alerts and do not report
security findings.

## Requirements and setup

Install a compatible CodeQL CLI separately and make its executable available to
LegacyLens through the configured CodeQL CLI path. The CLI downloads query-pack
dependencies when this pack is installed, so network access may be required.

From this directory, resolve the pack dependency once:

```text
codeql pack install
```

The `qlpack.yml` declares `codeql/java-all`. Keep this directory and its
`java/` queries together so CodeQL can resolve the Java libraries.

## Query result contracts

`java/calls.ql` returns scalar columns in this order:

1. caller name (`string`)
2. caller source path (`string`)
3. caller declaration line (`int`)
4. caller declaration column (`int`)
5. callee name (`string`)
6. callee source path (`string`)
7. callee declaration line (`int`)
8. callee declaration column (`int`)
9. call-site source path (`string`)
10. call-site line (`int`)
11. call-site column (`int`)

It includes direct `MethodCall` expressions with an enclosing method and a
resolved target method. Calls from constructors and unresolved calls are not
included.

`java/endpoints.ql` returns these scalar columns:

1. declaring class name (`string`)
2. method name (`string`)
3. source path (`string`)
4. declaration line (`int`)
5. declaration column (`int`)

The Servlet query uses CodeQL's `isServletRequestMethod` predicate. Its `path`
column is the source file path. The query does not infer an HTTP URL from servlet
annotations, descriptors, framework conventions, or runtime configuration.

## Run manually

Given a finalized Java database and an installed CodeQL CLI, a query can be
executed and decoded as follows. The forms follow the documented CLI synopsis,
but this machine does not have CodeQL installed, so no real query or decoded
JSON result has been validated here.

```text
codeql query run --database=<db> --output=<result.bqrs> -- <query.ql>
codeql bqrs decode --format=json --output=<result.json> -- <result.bqrs>
```

LegacyLens consumes the scalar table columns above. It does not treat these
informational query results as SARIF alerts.
