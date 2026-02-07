package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// dbMethodNames are the method names that typically execute SQL.
var dbMethodNames = map[string]bool{
	"Query":           true,
	"QueryRow":        true,
	"Exec":            true,
	"QueryContext":    true,
	"ExecContext":     true,
	"QueryRowContext": true,
	"Prepare":         true,
	"Get":             true,
	"Select":          true,
	"MustExec":        true,
	"NamedExec":       true,
	"NamedQuery":      true,
}

// GoScanner extracts SQL string literals from Go source files by walking
// the AST and looking for calls to known database methods.
type GoScanner struct{}

// Scan walks the given paths, finds .go files, and extracts SQL strings.
func (s *GoScanner) Scan(paths []string) ([]SQLStatement, error) {
	var results []SQLStatement

	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}

			stmts, err := scanGoFile(path)
			if err != nil {
				// Skip files that fail to parse (e.g. generated code)
				// so the scanner can continue with remaining files.
				return nil //nolint:nilerr // intentionally skip unparseable files
			}
			results = append(results, stmts...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// scanGoFile parses a single Go source file and extracts SQL string literals
// from calls to known database methods.
func scanGoFile(path string) ([]SQLStatement, error) {
	fset := token.NewFileSet()
	src, err := os.ReadFile(path) //nolint:gosec // scanning user-provided source paths
	if err != nil {
		return nil, err
	}

	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(src), "\n")
	directives := ParseDirectives(lines)

	var results []SQLStatement

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		methodName, ok := dbMethodName(call)
		if !ok {
			return true
		}

		// The SQL query is expected at a method-specific argument position.
		sqlArg := findSQLArg(call, methodName)
		if sqlArg == "" {
			return true
		}

		pos := fset.Position(call.Pos())
		line := pos.Line

		results = append(results, SQLStatement{
			SQL:      sqlArg,
			File:     path,
			Line:     line,
			Disabled: disabledRulesForLine(directives, line),
		})

		return true
	})

	return results, nil
}

// dbMethodName returns the database method name (Query, Exec, etc.) if this
// call expression targets a known database API method.
func dbMethodName(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if !dbMethodNames[sel.Sel.Name] {
		return "", false
	}
	return sel.Sel.Name, true
}

// sqlArgIndex returns the SQL argument index for a known database method.
// Context-based APIs place SQL at argument index 1, while others use index 0.
func sqlArgIndex(methodName string) int {
	switch methodName {
	case "QueryContext", "ExecContext", "QueryRowContext":
		return 1
	default:
		return 0
	}
}

// findSQLArg returns the SQL string literal at the expected SQL argument index
// for the given method. It handles both raw (backtick) and interpreted
// (double-quote) Go string literals.
func findSQLArg(call *ast.CallExpr, methodName string) string {
	idx := sqlArgIndex(methodName)
	if idx < 0 || idx >= len(call.Args) {
		return ""
	}

	lit, ok := call.Args[idx].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}

	// Remove quotes.
	val := lit.Value
	if strings.HasPrefix(val, "`") && strings.HasSuffix(val, "`") {
		return val[1 : len(val)-1]
	}
	if strings.HasPrefix(val, "\"") {
		unquoted, err := strconv.Unquote(val)
		if err == nil {
			return unquoted
		}
	}

	return ""
}
