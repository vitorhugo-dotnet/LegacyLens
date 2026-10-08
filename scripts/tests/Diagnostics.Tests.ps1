Describe 'Task 19 opt-in diagnostics' {
  BeforeAll {
    $root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
    $collector = Join-Path $root 'scripts/collect-diagnostics.ps1'
    $schemaPath = Join-Path $root 'docs/validation/result.schema.json'
  }

  It 'writes sanitized machine metadata without reading secrets or trace material' {
    $directory = Join-Path ([System.IO.Path]::GetTempPath()) ('legacylens-diagnostics-' + [guid]::NewGuid().ToString('N'))
    $output = Join-Path $directory 'diagnostics.json'
    $credential = Join-Path $directory 'credentials.json'
    New-Item -ItemType Directory -Path $directory | Out-Null
    Set-Content -LiteralPath $credential -Value '{"password":"credential-file-secret"}'
    $env:LEGACYLENS_DIAGNOSTIC_URL = 'https://example.invalid/api?token=url-token-secret'
    $env:LEGACYLENS_DIAGNOSTIC_EXCEPTION = "SQLException: SELECT * FROM orders WHERE note='sql-literal-secret'"
    $env:LEGACYLENS_DIAGNOSTIC_CREDENTIAL_FILE = $credential
    try {
      & $collector -OutputPath $output -Commit ('a' * 40)
      if (-not $?) { throw 'Diagnostics collector failed.' }
      $bundle = Get-Content -Raw -LiteralPath $output
      foreach ($secret in @('url-token-secret','sql-literal-secret','credential-file-secret','example.invalid','SELECT * FROM')) {
        if ($bundle.Contains($secret)) { throw "Sensitive value leaked into diagnostics: $secret" }
      }
      if ($bundle -notmatch '"sourceFilesIncluded"\s*:\s*false') { throw 'Diagnostics must declare that source files were not included.' }
    } finally {
      Remove-Item Env:LEGACYLENS_DIAGNOSTIC_URL -ErrorAction SilentlyContinue
      Remove-Item Env:LEGACYLENS_DIAGNOSTIC_EXCEPTION -ErrorAction SilentlyContinue
      Remove-Item Env:LEGACYLENS_DIAGNOSTIC_CREDENTIAL_FILE -ErrorAction SilentlyContinue
      Remove-Item -LiteralPath $directory -Recurse -Force -ErrorAction SilentlyContinue
    }
  }

  It 'never marks scenarios as passed unless an execution result is supplied' {
    $directory = Join-Path ([System.IO.Path]::GetTempPath()) ('legacylens-diagnostics-' + [guid]::NewGuid().ToString('N'))
    $output = Join-Path $directory 'diagnostics.json'
    New-Item -ItemType Directory -Path $directory | Out-Null
    try {
      & $collector -OutputPath $output -Commit ('b' * 40)
      if (-not $?) { throw 'Diagnostics collector failed.' }
      $bundle = Get-Content -Raw -LiteralPath $output | ConvertFrom-Json
      foreach ($scenario in $bundle.scenarios) {
        if ($scenario.status -eq 'passed') { throw "Scenario $($scenario.id) was incorrectly marked passed." }
        if ($scenario.status -ne 'not-run') { throw "Unexpected scenario state: $($scenario.status)" }
      }
    } finally { Remove-Item -LiteralPath $directory -Recurse -Force -ErrorAction SilentlyContinue }
  }

  It 'provides a schema for explicit validation results' {
    if (-not (Test-Path -LiteralPath $schemaPath)) { throw 'Validation result schema is missing.' }
    $schema = Get-Content -Raw -LiteralPath $schemaPath | ConvertFrom-Json
    if ($schema.required -notcontains 'commit' -or $schema.required -notcontains 'platform' -or $schema.required -notcontains 'scenarios') {
      throw 'Validation result schema omits required provenance fields.'
    }
  }
}
