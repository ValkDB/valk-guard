# Adding Rules in Valk Guard

This guide explains how to add a new lint rule to `valk-guard`.

## Rule Model

Each rule implements the `Rule` interface in `internal/rules/rule.go`:

- `ID() string`: unique ID like `VG009`
- `Name() string`: machine-friendly name
- `Description() string`: human-friendly summary
- `DefaultSeverity() Severity`: `error`, `warning`, or `info`
- `Check(...) []Finding`: rule logic

Rules run on parsed SQL (`postgresparser.ParsedQuery`) and return zero or more findings.

Optional optimization interface:

- `CommandTargetedRule` with `CommandTargets() []postgresparser.QueryCommand`

If implemented, `buildRulePlan(...)` runs that rule only for matching command types. If omitted, the rule is treated as cross-cutting.

## Step 1: Create the Rule

Add a new file under `internal/rules/`, for example:

- `internal/rules/vg009_no_select_distinct.go`

Implement a struct with methods matching `Rule`.

```go
type NoSelectDistinctRule struct{}

func (r *NoSelectDistinctRule) ID() string          { return "VG009" }
func (r *NoSelectDistinctRule) Name() string        { return "no-select-distinct" }
func (r *NoSelectDistinctRule) Description() string { return "Detects SELECT DISTINCT usage." }
func (r *NoSelectDistinctRule) DefaultSeverity() Severity { return SeverityWarning }
func (r *NoSelectDistinctRule) CommandTargets() []postgresparser.QueryCommand {
    return []postgresparser.QueryCommand{postgresparser.QueryCommandSelect}
}

func (r *NoSelectDistinctRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
    // rule logic here
    return nil
}
```

## Step 2: Register the Rule

Register the rule in `internal/rules/registry.go` inside `DefaultRegistry()`:

```go
mustRegister(reg, &NoSelectDistinctRule{})
```

Registration order controls output order when multiple rules fire.

## Step 3: Add Unit Tests

Create tests in a dedicated file, for example:

- `internal/rules/vg009_no_select_distinct_test.go`

Test both:

- Positive cases (finding expected)
- Negative cases (no finding expected)

Use real SQL strings parsed with `postgresparser.ParseSQL`.

## Step 4: Add Docs and Config Example

Update:

- `README.md` rule table and examples
- `.valk-guard.yaml.example` if useful

If severity/enable defaults are important for users, document them explicitly.

## Step 5: Validate

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
```

If `golangci-lint` is available in your environment, run:

```bash
golangci-lint run ./...
```

## Schema Rules (VG1xx)

Schema-drift rules implement the `SchemaRule` interface instead of `Rule`:

```go
type SchemaRule interface {
    ID() string
    Name() string
    Description() string
    DefaultSeverity() Severity
    CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding
}
```

Schema rules cross-reference ORM model definitions (extracted from Go struct `db` tags or Python `__tablename__`/`Column()`) against migration DDL (parsed via `postgresparser`). They run after the per-statement phase.

### Adding a Schema Rule

1. Create `internal/rules/vg1xx_your_rule.go` implementing `SchemaRule`.
2. Register in `DefaultRegistry()` using `mustRegisterSchema(reg, &YourRule{})`.
3. Schema rules receive a `*schema.Snapshot` (accumulated DDL state) and `[]schema.ModelDef` (extracted models).
4. Use `matchTable(snap, modelTable)` to resolve model table names against the snapshot (exact case-insensitive matching).
5. Respect model metadata in `schema.ModelDef`:
   - `Source` identifies model engine (`go` or `sqlalchemy`).
   - `TableExplicit` identifies whether table mapping is explicit in source (for example `__tablename__`).
   - `TableMappingKind` / `TableMappingSource` identify inferred vs explicit table mapping provenance.
   - `ModelColumn.MappingKind` / `MappingSource` identify inferred vs explicit column mapping provenance.

See `vg101_dropped_column.go`, `vg109_orphan_migration_table.go`, and `vg111_go_inferred_table_name_risk.go` for reference implementations.

## Query-Schema Rules (VG105+)

Query-schema rules compare parsed query column usage with schema snapshots selected by runtime (migration DDL and, when available, engine-matched model snapshots).
They implement `QuerySchemaRule` in `internal/rules/query_schema_rule.go`:

```go
type QuerySchemaRule interface {
    ID() string
    Name() string
    Description() string
    DefaultSeverity() Severity
    CheckQuerySchema(snap *schema.Snapshot, stmt scanner.SQLStatement, parsed *postgresparser.ParsedQuery) []Finding
}
```

### Adding a Query-Schema Rule

1. Create `internal/rules/vg10x_your_rule.go` implementing `QuerySchemaRule`.
2. Register in `DefaultRegistry()` using `mustRegisterQuerySchema(reg, &YourRule{})`.
3. Use parser metadata (`parsed.Tables`, `parsed.ColumnUsage`) plus schema snapshot tables to resolve unknown columns.
4. Respect statement metadata in `scanner.SQLStatement`:
   - `Engine` for per-rule engine scoping (`sql`, `go`, `goqu`, `sqlalchemy`)
   - `Disabled` for inline suppression directives

`VG105`-`VG108` are reference implementations for projection, predicate, table-reference, and ambiguity checks across migration and model-derived snapshots.

## Design Tips

- Keep checks deterministic and parser-driven, not regex-only where possible.
- Prefer low false-positive logic over aggressive detection.
- Set `Column: 1` unless you have accurate column metadata.
- Return concise, actionable finding messages.
- Avoid rule overlap unless intentionally complementary.
