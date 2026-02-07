# Adding Scanners (ORM/Languages)

This guide shows how to extend Valk Guard with new SQL extractors (for ORMs or new languages) without changing the rule engine or reporters.

## Architecture Contract

Valk Guard flow is:

1. Scanners emit `scanner.SQLStatement`
2. Parser converts SQL text to `postgresparser.ParsedQuery`
3. Rules evaluate parsed queries into `rules.Finding`
4. Reporters render findings

As long as your scanner outputs `SQLStatement`, the rest of the pipeline is reused.

## Step 1: Implement `scanner.Scanner`

Create a new scanner file, for example:

- `scanner/gorm_scanner.go`
- `scanner/sqlalchemy_scanner.go`

Implement:

```go
type GORMScanner struct{}

func (s *GORMScanner) Scan(paths []string) ([]SQLStatement, error) {
    // discover source files
    // parse AST / syntax tree
    // extract SQL or SQL-like query text
    // map each statement to file + line
    return nil, nil
}
```

## Step 2: Reuse Inline Suppression Handling

For files with comments/directives:

- Split source by lines
- Use `ParseDirectives(lines)`
- Attach disables with `disabledRulesForLine(...)`

This keeps behavior consistent with existing SQL/Go scanners.

## Step 3: Register the Scanner

Add the scanner in:

- `cmd/valk-guard/main.go` via `configuredScanners()`

Example:

```go
{label: "GORM calls", impl: &scanner.GORMScanner{}},
```

No rule/reporter changes are required.

## ORM Strategy

Prefer this order:

1. Extract raw SQL when ORM exposes it directly.
2. If not available, extract stable SQL templates (`SELECT ... WHERE id = ?`).
3. Avoid overly inferred SQL that can create false positives.

If a framework cannot produce reliable SQL text, add scanner support later with an explicit confidence threshold.

## Testing

Add scanner tests similar to existing scanner tests:

- fixture source files in `testdata/`
- positive extraction cases
- no-SQL cases
- line number and suppression behavior

Run:

```bash
go test ./scanner -v
go test ./... 
```
