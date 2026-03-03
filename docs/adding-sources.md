# Adding Sources (Scanners + Models)

This guide shows how to add a new source framework (for example GORM) so Valk Guard can lint its SQL and, when available, use model metadata for schema-aware rules.

## Source Architecture

A source integration can include:

1. SQL scanner that emits `scanner.SQLStatement`.
2. Model extractor that emits generic `schema.ModelDef`.

Runtime wiring is registry-based:

- scanner bindings: `defaultScannerBindings()` in `cmd/valk-guard/source_bindings.go`
- model bindings: `defaultModelBindings(cfg)` in `cmd/valk-guard/source_bindings.go`

`collectAndAnalyze()` consumes those bindings, so adding a source does not require new hardcoded switches in `main.go`.

## Core Contracts

### SQL Scanner

Implement `scanner.Scanner`:

```go
type Scanner interface {
    Scan(ctx context.Context, paths []string) iter.Seq2[SQLStatement, error]
}
```

Set `SQLStatement.SQL`, `File`, `Line`, `Engine`, and `Disabled`.

### Model Extractor

Implement `schema.ModelExtractor`:

```go
type ModelExtractor interface {
    ExtractModels(ctx context.Context, paths []string) ([]ModelDef, error)
}
```

Normalize framework metadata to:

- `ModelDef.Table`, `ModelDef.Source`, `ModelDef.Columns`
- `ModelColumn.Name`, `ModelColumn.Type`, `ModelColumn.Field`
- `ModelColumn.MappingKind` / `MappingSource` for explicit vs inferred provenance
- `ModelDef.TableMappingKind` / `TableMappingSource` for explicit vs inferred table provenance

Provenance contract:

- Use `explicit` when mapping comes from declared framework metadata.
- Use `inferred` when mapping comes from fallback naming.
- Populate `...MappingSource` with a stable provider token (`<source>.<provider>`) so rule logic can stay source-agnostic.

## Integration Steps

### 1) Add Engine Constant

Add a new engine in `internal/scanner/scanner.go`, for example:

```go
const EngineGorm Engine = "gorm"
```

Then add it to built-in engine allowlist in `internal/scanner/engines.go` (`knownEngines`), so config engine validation accepts it.

### 2) Add Scanner Package

Create `internal/scanner/<source>/...` implementing `Scan(...)`.

Examples:

- `internal/scanner/goqu/goqu_scanner.go`
- `internal/scanner/sqlalchemy/sqlalchemy_scanner.go`

### 3) Register Scanner Binding

Add an entry in `defaultScannerBindings()`:

```go
{
    name: "gorm",
    impl: &gormscanner.Scanner{},
    extensions: []string{".go"},
}
```

File discovery is automatic from registered extensions via `requiredExtensions(...)` and `collectScannerInputs(...)`.

### 4) Optional: Add Model Extractor

If the source has model metadata, add `internal/schema/<source>/...` implementing `schema.ModelExtractor`.

For Go sources, model extraction mode is configurable:

```yaml
go_model:
  mapping_mode: strict # strict | balanced | permissive
```

When writing the extractor, emit normalized mapping provenance so existing schema rules can work without source-specific branches:

- explicit column mappings: `ModelColumn.MappingKind = explicit`
- inferred column mappings: `ModelColumn.MappingKind = inferred`
- explicit table mappings: `ModelDef.TableMappingKind = explicit`
- inferred table mappings: `ModelDef.TableMappingKind = inferred`

### 5) Register Model Binding

Add an entry in `defaultModelBindings()`:

```go
{
    source:        schema.ModelSourceGorm,
    extractor:     &gormmodel.Extractor{},
    extensions:    []string{".go"},
    configEngines: []scanner.Engine{scanner.EngineGorm},
    queryEngines:  []scanner.Engine{scanner.EngineGorm},
}
```

Binding fields control behavior:

- `configEngines`: which `rules.<id>.engines` values enable schema rules (`VG101`-`VG104`) for this model source.
- `queryEngines`: which statement engines should include this source's model snapshot for query-schema rules (`VG105`-`VG108`).

## How Rule Families Use Source Bindings

1. `VG001`-`VG008`: run on parsed statements emitted by scanners.
2. `VG101`-`VG104`: run on migration snapshot + models filtered by `configEngines`.
3. `VG105`-`VG108`: run on migration snapshot plus model snapshots mapped by `queryEngines`.

## Required Tests

1. Scanner package unit tests.
2. Model extractor unit tests (if extractor added).
3. `cmd/valk-guard/main_test.go` integration tests:
   - source statements are detected
   - engine scoping via `rules.<id>.engines` works
   - `VG105`/`VG106` include `WHERE`, `INNER JOIN ... ON ...`, and grouping/sort coverage
   - model-aware cases when extractor is wired

## Docs to Update

1. `README.md` engine/rule support matrix.
2. `docs/schema-drift.md` source-to-model snapshot mapping.
3. `.valk-guard.yaml.example` engine examples.

## Quick Checklist

1. Engine constant added.
2. Engine listed in `internal/scanner/engines.go`.
3. Scanner implemented and bound.
4. Optional extractor implemented and model-bound.
5. Tests added for scanner/query-schema/schema-drift behavior.
6. Docs updated.
7. `go test ./...` passes.
