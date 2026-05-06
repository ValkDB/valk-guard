# Source Configuration

Valk Guard enables every source scanner by default. Use top-level `sources` in
`.valk-guard.yaml` to disable whole integrations before file discovery and
external runtime startup.

```yaml
sources:
  sql: true          # .sql files
  go: true           # database/sql-style Go string scanning
  goqu: true         # Goqu AST synthetic SQL
  sqlalchemy: true   # Python SQLAlchemy AST synthetic SQL
  csharp: true       # C# EF Core Roslyn synthetic SQL
```

Missing entries default to `true`, so this is enough to turn off C# while keeping
everything else enabled:

```yaml
sources:
  csharp: false
```

To turn off all Go source scanning, disable both Go integrations:

```yaml
sources:
  go: false
  goqu: false
```

`go` controls database/sql-style Go string scanning. `goqu` controls Goqu AST
synthetic SQL. If either is enabled, `.go` files may still be discovered because
that enabled source needs them.

## Aliases

The config accepts these aliases and normalizes them internally:

| Alias | Source |
| --- | --- |
| `python`, `py` | `sqlalchemy` |
| `cs`, `c#`, `dotnet` | `csharp` |

Example:

```yaml
sources:
  python: false
  cs: false
```

## Difference From Rule Engine Filters

`sources` disables scanner/model-extractor integrations entirely. Disabled
sources do not collect files, do not invoke Python or .NET, and do not emit SQL
statements.

`rules.<id>.engines` only filters findings after SQL has already been scanned
and parsed:

```yaml
rules:
  VG001:
    engines: [sql, goqu]
```

Use `sources` for runtime/control-plane behavior. Use `rules.<id>.engines` for
rule noise tuning.

## Runtime Notes

- SQL, Go, and Goqu scanning run in the Valk Guard binary.
- SQLAlchemy scanning invokes Python only when `.py` candidates contain
  SQLAlchemy markers.
- C# scanning invokes `dotnet` and the embedded Roslyn extractor when `.cs`
  files are scanned. Roslyn decides from parsed syntax whether EF Core SQL or
  deterministic LINQ chains are present; the Go wrapper does not substring-match
  C# code constructs.
