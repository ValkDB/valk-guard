# Adding Sources (Scanners + Models)

This guide shows how to add a new source framework (for example GORM) so Valk Guard can lint its SQL and, when possible, use its model metadata.

## What "Source" Means

A source integration can include one or both parts:

1. SQL scanner: emits `scanner.SQLStatement` from source files.
2. Model extractor: emits generic `schema.ModelDef` / `schema.ModelColumn`.

Rule families use those generic contracts:

- `VG001`-`VG008`: parsed SQL only.
- `VG105`-`VG106`: parsed SQL + schema snapshots (migrations and engine-matched model snapshots when available).
- `VG101`-`VG104`: model-vs-migration checks.

## Core Contracts

### SQL Scanner Contract

Implement `scanner.Scanner`:

```go
type Scanner interface {
    Scan(ctx context.Context, paths []string) iter.Seq2[SQLStatement, error]
}
```

Each emitted `SQLStatement` should include:

- `SQL`
- `File`
- `Line`
- `Engine`
- `Disabled` (inline suppression IDs)

Reference: `internal/scanner/scanner.go`.

### Model Extractor Contract

Implement `schema.ModelExtractor`:

```go
type ModelExtractor interface {
    ExtractModels(ctx context.Context, paths []string) ([]ModelDef, error)
}
```

Normalize framework metadata into generic models:

- `ModelDef.Table`, `ModelDef.Source`, `ModelDef.Columns`
- `ModelColumn.Name`, `ModelColumn.Type`, `ModelColumn.Field`

Reference: `internal/schema/model.go`.

## Step-by-Step Integration

### 1) Add Scanner Package

Create `internal/scanner/<source>/...` and implement `Scan(...)`.

Keep scanner responsibilities narrow:

- discover candidate files
- extract SQL text
- map file/line correctly
- attach inline suppressions via `ParseDirectives(...)` + `DisabledRulesForLine(...)`

Examples:

- `internal/scanner/goqu/goqu_scanner.go`
- `internal/scanner/sqlalchemy/sqlalchemy_scanner.go`

### 2) Add Engine Identifier

Add a new engine constant in `internal/scanner/scanner.go`, for example:

```go
const EngineGorm Engine = "gorm"
```

Then allow it in config engine validation (`internal/config/config.go`), so rule scoping supports:

```yaml
rules:
  VG105:
    engines: [gorm]
```

### 3) Wire Scanner in Runtime

Register scanner in `activeScanners(...)` in `cmd/valk-guard/main.go`.

Also ensure file collection includes the relevant extension(s) if needed.

### 4) Optional: Add Model Extractor

If the source exposes model metadata (recommended), add extractor under `internal/schema/<source>/...`.

Extractor output must be generic `schema.ModelDef`.

This enables:

- `VG101`-`VG104` if you wire model-source mapping for schema rules.
- richer `VG105`/`VG106` checks when query-schema runtime maps that engine to a model snapshot.

### 5) Map Source to Query-Schema Model Snapshot

In `runSchemaDrift(...)` (`cmd/valk-guard/main.go`), extend engine-to-snapshot mapping in `querySnapshotsForEngine(...)`.

Example intent:

- `goqu` / `go` -> Go model snapshot
- `sqlalchemy` -> SQLAlchemy model snapshot
- `gorm` -> GORM model snapshot

### 6) Tests (Required)

Add integration tests in `cmd/valk-guard/main_test.go`:

- scanner emits statements
- `VG001`-`VG008` run on source SQL
- `VG105`/`VG106` findings include `WHERE` + `INNER JOIN ... ON ...` cases
- engine filtering works (`rules.<id>.engines`)

If extractor exists, add tests proving model-aware behavior:

- migration allows column, model omits it -> `VG105`/`VG106` should still flag when model snapshot applies

Add unit tests for scanner/extractor packages.

### 7) Documentation Updates

Update:

- `README.md` rule/support matrix if engine support expands
- `docs/schema-drift.md` schema-aware behavior/source mapping
- `.valk-guard.yaml.example` engine examples

## Design Guardrails

1. Keep false positives low. Prefer conservative extraction over speculative SQL.
2. Emit best-effort SQL shape if raw SQL is unavailable, but make structure stable.
3. Always preserve file/line mapping quality.
4. Keep rules framework-agnostic. Framework logic belongs in scanner/extractor/runtime mapping.

## Quick Checklist

1. New scanner implemented.
2. Engine constant + config validation updated.
3. Runtime scanner registration done.
4. Optional model extractor implemented and mapped.
5. Tests added (including `INNER JOIN` coverage for `VG106`).
6. Docs/support matrix updated.
7. `go test ./...` passes.
