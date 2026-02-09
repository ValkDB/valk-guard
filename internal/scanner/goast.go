package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GoFileVisitor is the callback signature for WalkGoFiles. It receives the
// file path, the token file set, the parsed AST, and the raw source bytes.
type GoFileVisitor func(path string, fset *token.FileSet, file *ast.File, src []byte) error

// WalkGoFiles walks the given root paths, finds .go files, parses each into
// an AST, and invokes fn for every successfully parsed file.
// Read and parse errors are returned with file context.
func WalkGoFiles(ctx context.Context, paths []string, fn GoFileVisitor) error {
	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}

			fset := token.NewFileSet()
			src, err := os.ReadFile(path) //nolint:gosec // scanning user-provided source paths
			if err != nil {
				return fmt.Errorf("reading go file %s: %w", path, err)
			}

			f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parsing go file %s: %w", path, err)
			}

			return fn(path, fset, f, src)
		})
		if err != nil {
			return fmt.Errorf("walking go paths: %w", err)
		}
	}
	return nil
}

// ExtractStringLiteral extracts the Go string value from a *ast.BasicLit.
// It handles both raw (backtick) and interpreted (double-quote) literals.
// Returns the unquoted string, or "" if the literal cannot be decoded.
func ExtractStringLiteral(lit *ast.BasicLit) string {
	if lit == nil || lit.Kind != token.STRING {
		return ""
	}

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

// FindImportAlias inspects the import declarations of a Go file and returns
// the local alias used for one of the given import paths. If no matching
// import is found, it returns "". If the import is unnamed, it returns the
// last segment of the import path as the default alias.
func FindImportAlias(file *ast.File, importPaths map[string]bool) string {
	for _, imp := range file.Imports {
		unquoted, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if !importPaths[unquoted] {
			continue
		}

		// Explicit alias: import myalias "github.com/..."
		if imp.Name != nil {
			return imp.Name.Name
		}

		// Default alias: last segment of the path, stripping /vN suffixes.
		parts := strings.Split(unquoted, "/")
		alias := parts[len(parts)-1]
		if len(parts) > 1 && strings.HasPrefix(alias, "v") {
			if _, err := strconv.Atoi(alias[1:]); err == nil {
				alias = parts[len(parts)-2]
			}
		}
		return alias
	}
	return ""
}
