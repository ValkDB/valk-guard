package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// TestWalkGoFilesFindsGoFiles verifies that WalkGoFiles visits .go files
// and skips non-.go files.
func TestWalkGoFilesFindsGoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a .go file and a .txt file.
	goFile := filepath.Join(tmpDir, "main.go")
	txtFile := filepath.Join(tmpDir, "notes.txt")

	if err := os.WriteFile(goFile, []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatalf("failed to write .go file: %v", err)
	}
	if err := os.WriteFile(txtFile, []byte("not go"), 0644); err != nil {
		t.Fatalf("failed to write .txt file: %v", err)
	}

	var visited []string
	err := WalkGoFiles([]string{tmpDir}, func(path string, fset *token.FileSet, file *ast.File, src []byte) error {
		visited = append(visited, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkGoFiles error: %v", err)
	}

	if len(visited) != 1 {
		t.Fatalf("expected 1 visited file, got %d: %v", len(visited), visited)
	}
	if visited[0] != goFile {
		t.Errorf("expected %s, got %s", goFile, visited[0])
	}
}

// TestWalkGoFilesSkipsUnparseableFiles verifies that files with syntax errors
// are silently skipped.
func TestWalkGoFilesSkipsUnparseableFiles(t *testing.T) {
	tmpDir := t.TempDir()

	badFile := filepath.Join(tmpDir, "bad.go")
	if err := os.WriteFile(badFile, []byte("this is not valid go"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	var visited int
	err := WalkGoFiles([]string{tmpDir}, func(path string, fset *token.FileSet, file *ast.File, src []byte) error {
		visited++
		return nil
	})
	if err != nil {
		t.Fatalf("WalkGoFiles error: %v", err)
	}

	if visited != 0 {
		t.Errorf("expected 0 visited files for unparseable input, got %d", visited)
	}
}

// TestExtractStringLiteralDoubleQuoted verifies extraction of "..." strings.
func TestExtractStringLiteralDoubleQuoted(t *testing.T) {
	lit := &ast.BasicLit{Kind: token.STRING, Value: `"SELECT 1"`}
	got := ExtractStringLiteral(lit)
	if got != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got %q", got)
	}
}

// TestExtractStringLiteralBacktick verifies extraction of `...` strings.
func TestExtractStringLiteralBacktick(t *testing.T) {
	lit := &ast.BasicLit{Kind: token.STRING, Value: "`SELECT 1`"}
	got := ExtractStringLiteral(lit)
	if got != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got %q", got)
	}
}

// TestExtractStringLiteralNil verifies that nil input returns "".
func TestExtractStringLiteralNil(t *testing.T) {
	got := ExtractStringLiteral(nil)
	if got != "" {
		t.Errorf("expected empty string for nil, got %q", got)
	}
}

// TestExtractStringLiteralNonString verifies that non-STRING tokens return "".
func TestExtractStringLiteralNonString(t *testing.T) {
	lit := &ast.BasicLit{Kind: token.INT, Value: "42"}
	got := ExtractStringLiteral(lit)
	if got != "" {
		t.Errorf("expected empty string for INT literal, got %q", got)
	}
}

// TestFindImportAliasDefault verifies that unnamed imports return the last path segment.
func TestFindImportAliasDefault(t *testing.T) {
	src := `package main
import "github.com/doug-martin/goqu/v9"
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	paths := map[string]bool{"github.com/doug-martin/goqu/v9": true}
	alias := FindImportAlias(f, paths)
	if alias != "v9" {
		t.Errorf("expected alias 'v9', got %q", alias)
	}
}

// TestFindImportAliasExplicit verifies that explicitly named imports return the alias.
func TestFindImportAliasExplicit(t *testing.T) {
	src := `package main
import mygoqu "github.com/doug-martin/goqu/v9"
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	paths := map[string]bool{"github.com/doug-martin/goqu/v9": true}
	alias := FindImportAlias(f, paths)
	if alias != "mygoqu" {
		t.Errorf("expected alias 'mygoqu', got %q", alias)
	}
}

// TestFindImportAliasNotFound verifies that non-imported paths return "".
func TestFindImportAliasNotFound(t *testing.T) {
	src := `package main
import "fmt"
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	paths := map[string]bool{"github.com/doug-martin/goqu": true}
	alias := FindImportAlias(f, paths)
	if alias != "" {
		t.Errorf("expected empty alias, got %q", alias)
	}
}
