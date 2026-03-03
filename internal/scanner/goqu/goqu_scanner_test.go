package goqu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestGoquScannerExtractsRawAndSyntheticSQL(t *testing.T) {
	src := `package example

import goqu "github.com/doug-martin/goqu/v9"

func queries() {
	goqu.L("SELECT * FROM users")
	goqu.From("users").Join(goqu.T("orders"), goqu.On(goqu.Ex{"users.id": goqu.I("orders.uid")})).Select(goqu.Star())
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "goqu_usage.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) < 2 {
		t.Fatalf("expected at least 2 SQL statements, got %d: %+v", len(stmts), stmts)
	}

	if !hasSQL(stmts, "SELECT * FROM users") {
		t.Fatalf("expected raw goqu.L SQL to be extracted")
	}

	if !hasSQLContaining(stmts, "/* valk-guard:synthetic goqu-ast */ SELECT * FROM users JOIN orders ON 1=1") {
		t.Fatalf("expected synthetic SQL with JOIN and SELECT * from builder chain, got %+v", stmts)
	}
}

func TestGoquScannerBuilderChainsTriggerRules(t *testing.T) {
	src := `package example

import goqu "github.com/doug-martin/goqu/v9"

func queries() {
	goqu.From("users").LeftJoin(goqu.T("orders"), goqu.On(goqu.Ex{"users.id": goqu.I("orders.uid")})).Select(goqu.Star())
	goqu.From("users").Join(goqu.T("profiles"), goqu.On(goqu.Ex{"users.id": goqu.I("profiles.uid")})).Where(goqu.C("email").Like("%@gmail.com")).Select("users.id")
	goqu.From("users").ForUpdate().Select("id")
	goqu.From("logs").LeftJoin(goqu.T("users"), goqu.On(goqu.Ex{"logs.uid": goqu.I("users.id")})).Select("logs.id", "logs.msg")
	goqu.Update("inventory").Set(goqu.Record{"stock": 0})
	goqu.Delete("sessions")
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "goqu_builder.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	findingsByRule := collectFindingsByRule(t, stmts)

	requiredRules := []string{"VG001", "VG002", "VG003", "VG004", "VG005", "VG006"}
	for _, ruleID := range requiredRules {
		if findingsByRule[ruleID] == 0 {
			t.Fatalf("expected %s finding from builder chains, got none (all findings: %+v)", ruleID, findingsByRule)
		}
	}

	if !hasSQLContaining(stmts, "LEFT JOIN orders ON 1=1") {
		t.Fatalf("expected synthetic SQL to preserve join structure, got %+v", stmts)
	}
	if !hasSQLContaining(stmts, "email LIKE '%@gmail.com'") {
		t.Fatalf("expected synthetic SQL to preserve LIKE predicate, got %+v", stmts)
	}
	if !hasSQLContaining(stmts, "FOR UPDATE") {
		t.Fatalf("expected synthetic SQL to preserve FOR UPDATE, got %+v", stmts)
	}
}

func TestGoquScannerDirectiveSuppressionOnSyntheticSQL(t *testing.T) {
	src := `package example

import goqu "github.com/doug-martin/goqu/v9"

func queries() {
	// valk-guard:disable VG004
	goqu.From("logs").LeftJoin(goqu.T("users"), goqu.On(goqu.Ex{"logs.uid": goqu.I("users.id")})).Select("logs.id", "logs.msg")
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "directive.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %+v", len(stmts), stmts)
	}
	if len(stmts[0].Disabled) != 1 || stmts[0].Disabled[0] != "VG004" {
		t.Fatalf("expected disabled=[VG004], got %v", stmts[0].Disabled)
	}
	if !strings.HasPrefix(stmts[0].SQL, "/* valk-guard:synthetic goqu-ast */") {
		t.Fatalf("expected synthetic marker prefix, got %q", stmts[0].SQL)
	}
}

func TestGoquScannerSkipsWithoutImport(t *testing.T) {
	src := `package example

func foo() {
	goqu.From("users").Select("*")
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "no_import.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements without goqu import, got %d: %+v", len(stmts), stmts)
	}
}

func TestGoquScannerEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}
}

func hasSQL(stmts []scanner.SQLStatement, want string) bool {
	for _, stmt := range stmts {
		if stmt.SQL == want {
			return true
		}
	}
	return false
}

func hasSQLContaining(stmts []scanner.SQLStatement, want string) bool {
	for _, stmt := range stmts {
		if strings.Contains(stmt.SQL, want) {
			return true
		}
	}
	return false
}

func collectFindingsByRule(t *testing.T, stmts []scanner.SQLStatement) map[string]int {
	t.Helper()

	findingsByRule := make(map[string]int)
	reg := rules.DefaultRegistry()

	for _, stmt := range stmts {
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			t.Fatalf("failed to parse SQL %q: %v", stmt.SQL, err)
		}
		if parsed == nil {
			continue
		}
		for _, rule := range reg.All() {
			finds := rule.Check(parsed, stmt.File, stmt.Line, stmt.SQL)
			if len(finds) > 0 {
				findingsByRule[rule.ID()] += len(finds)
			}
		}
	}

	return findingsByRule
}
