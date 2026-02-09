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

## Design Tips

- Keep checks deterministic and parser-driven, not regex-only where possible.
- Prefer low false-positive logic over aggressive detection.
- Set `Column: 1` unless you have accurate column metadata.
- Return concise, actionable finding messages.
- Avoid rule overlap unless intentionally complementary.
