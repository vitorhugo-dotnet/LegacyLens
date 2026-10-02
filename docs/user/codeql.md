# Optional CodeQL enrichment

LegacyLens can optionally use the CodeQL CLI to add Java call relationships and
conservative Servlet handler information to a local analysis. CodeQL is an
external, separately installed tool; the normal LegacyLens workflow does not
require it. If the CLI is unavailable, this enrichment is skipped and the
regular static analyzers continue to provide their results.

## Enable it deliberately

CodeQL database extraction runs project build commands and reads source files.
Enable this feature only for a project you are authorized to analyze. Review the
project root and the exact build argument vector before starting extraction.
LegacyLens passes build arguments as individual argv entries to
`codeql database trace-command`; it does not compose them into a shell command.
The configured timeout bounds the CLI operation. A timeout or CLI failure is
reported as an unavailable optional enrichment rather than a failure of the
whole local analysis.

CodeQL creates a local database from the selected Java project and executes the
two queries in [`analysis/codeql`](../../analysis/codeql/README.md). The data is
used locally to enrich the investigation graph. It is not uploaded by this
workflow.

## Current CLI sequence

The adapter uses the following CodeQL CLI sequence. Replace placeholders with
the configured database path, project root, build argv entries, and query path.
The database setup/build sequence is used only when the adapter needs to create
the database; an existing database can go directly to query execution.

```text
codeql database init --language=java --source-root=<projectRoot> -- <dbPath>
codeql database trace-command -- <dbPath> -- <build argv...>
codeql database finalize -- <dbPath>
codeql query run --database=<db> --output=<result.bqrs> -- <query.ql>
codeql bqrs decode --format=json --output=<result.json> -- <result.bqrs>
```

`<build argv...>` means a sequence of separate arguments, for example an
executable followed by its arguments. Do not join them into a shell string.
The command forms follow the CodeQL CLI documented synopsis, but they have not
been exercised here because the CodeQL CLI is not installed. The decoded JSON
shape is also unverified against a real CLI result. Treat this integration as
experimental until the queries and adapter output are exercised with a real
CodeQL installation and Java database.

## What the queries report

- **Calls** lists direct Java `MethodCall` sites for which CodeQL resolves an
  enclosing method and target method. Each row includes method names, source
  paths, declaration positions, and call-site position.
- **Servlet handlers** uses CodeQL's official
  `isServletRequestMethod` predicate. The reported path is the source file
  path; a URL mapping is not inferred.

These are informational tables, not SARIF alerts or security findings. Results
can be incomplete when extraction cannot observe the project's build, when a
call target cannot be resolved, or when endpoint routing is defined outside the
Servlet request-handler convention recognized by CodeQL.
