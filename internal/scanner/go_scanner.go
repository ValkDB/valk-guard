package scanner

import (
	"context"
	"errors"
	"go/ast"
	"go/token"
	"iter"
	"strings"
)

type dbMethodSpec struct {
	sqlArgIndex int
}

// dbMethodSpecs maps database execution methods to their SQL argument index.
// This avoids confusing builder-style methods with execution methods.
var dbMethodSpecs = map[string]dbMethodSpec{
	"Query":           {sqlArgIndex: 0},
	"QueryRow":        {sqlArgIndex: 0},
	"Exec":            {sqlArgIndex: 0},
	"QueryContext":    {sqlArgIndex: 1},
	"ExecContext":     {sqlArgIndex: 1},
	"QueryRowContext": {sqlArgIndex: 1},
	"Prepare":         {sqlArgIndex: 0},
	"Get":             {sqlArgIndex: 1},
	"Select":          {sqlArgIndex: 1},
	"MustExec":        {sqlArgIndex: 0},
	"NamedExec":       {sqlArgIndex: 0},
	"NamedQuery":      {sqlArgIndex: 0},
}

// GoScanner extracts SQL string literals from Go source files by walking
// the AST and looking for calls to known database methods.
type GoScanner struct{}

var errGoScannerStop = errors.New("go scanner stop")

// Scan walks the given paths, finds .go files, and streams SQL strings.
func (s *GoScanner) Scan(ctx context.Context, paths []string) iter.Seq2[SQLStatement, error] {
	return func(yield func(SQLStatement, error) bool) {
		err := WalkGoFiles(ctx, paths, func(path string, fset *token.FileSet, f *ast.File, src []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			lines := strings.Split(string(src), "\n")
			directives := ParseDirectives(lines)

			stop := false
			ast.Inspect(f, func(n ast.Node) bool {
				if stop {
					return false
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				spec, ok := dbMethodSpecForCall(call)
				if !ok {
					return true
				}

				sqlArg := findSQLArg(call, spec)
				if sqlArg == "" || !looksLikeSQL(sqlArg) {
					return true
				}

				pos := fset.Position(call.Pos())
				line := pos.Line

				if !yield(SQLStatement{
					SQL:      sqlArg,
					File:     path,
					Line:     line,
					Disabled: DisabledRulesForLine(directives, line),
				}, nil) {
					stop = true
					return false
				}
				return true
			})

			if stop {
				return errGoScannerStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errGoScannerStop) {
			_ = yield(SQLStatement{}, err)
		}
	}
}

// dbMethodSpecForCall returns the scanner spec for a known database call.
func dbMethodSpecForCall(call *ast.CallExpr) (dbMethodSpec, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return dbMethodSpec{}, false
	}
	spec, ok := dbMethodSpecs[sel.Sel.Name]
	if !ok {
		return dbMethodSpec{}, false
	}
	return spec, true
}

// findSQLArg returns the SQL string literal at the expected SQL argument index
// for the given method. It handles both raw (backtick) and interpreted
// (double-quote) Go string literals.
func findSQLArg(call *ast.CallExpr, spec dbMethodSpec) string {
	idx := spec.sqlArgIndex
	if idx < 0 || idx >= len(call.Args) {
		return ""
	}

	lit, ok := call.Args[idx].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}

	return ExtractStringLiteral(lit)
}

// looksLikeSQL reports whether the string appears to be a SQL statement
// by checking for common starting keywords or comments.
func looksLikeSQL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Fast path: check for common statement starts.
	upper := strings.ToUpper(s)
	keywords := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE",
		"DROP", "ALTER", "TRUNCATE", "WITH", "GRANT",
		"REVOKE", "BEGIN", "COMMIT", "ROLLBACK", "SET",
		"COPY", "VACUUM", "ANALYZE", "EXPLAIN", "MERGE",
	}
	for _, k := range keywords {
		if strings.HasPrefix(upper, k) {
			return true
		}
	}
	// Also allow comments as they are valid SQL starts.
	return strings.HasPrefix(s, "--") || strings.HasPrefix(s, "/*")
}
