// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package goqu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scannertest"
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

	if !scannertest.HasSQL(stmts, "SELECT * FROM users") {
		t.Fatalf("expected raw goqu.L SQL to be extracted")
	}

	if !scannertest.HasSQLContaining(stmts, "/* valk-guard:synthetic goqu-ast */ SELECT * FROM users JOIN orders ON 1=1") {
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

	findingsByRule := scannertest.CollectFindingsByRule(t, stmts)

	requiredRules := []string{"VG001", "VG002", "VG003", "VG004", "VG005", "VG006"}
	for _, ruleID := range requiredRules {
		if findingsByRule[ruleID] == 0 {
			t.Fatalf("expected %s finding from builder chains, got none (all findings: %+v)", ruleID, findingsByRule)
		}
	}

	if !scannertest.HasSQLContaining(stmts, "LEFT JOIN orders ON 1=1") {
		t.Fatalf("expected synthetic SQL to preserve join structure, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "email LIKE '%@gmail.com'") {
		t.Fatalf("expected synthetic SQL to preserve LIKE predicate, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "FOR UPDATE") {
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

func TestGoquScannerAliasedImport(t *testing.T) {
	src := `package example

import db "github.com/doug-martin/goqu/v9"

func queries() {
	db.From("users").Select("id", "email").Where(db.C("active").Eq(true)).Limit(10)
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "aliased.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) == 0 {
		t.Fatal("expected statements from aliased goqu import, got 0")
	}
	if !scannertest.HasSQLContaining(stmts, "/* valk-guard:synthetic goqu-ast */") {
		t.Fatalf("expected synthetic SQL from aliased import, got %+v", stmts)
	}
}

func TestGoquScannerMultiJoinChain(t *testing.T) {
	src := `package example

import goqu "github.com/doug-martin/goqu/v9"

func queries() {
	goqu.From("users").
		Join(goqu.T("orders"), goqu.On(goqu.Ex{"users.id": goqu.I("orders.uid")})).
		Join(goqu.T("items"), goqu.On(goqu.Ex{"orders.id": goqu.I("items.oid")})).
		Select("users.id", "orders.total", "items.name").
		Where(goqu.C("users.active").Eq(true)).
		Limit(100)
}
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "multi_join.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) == 0 {
		t.Fatal("expected statements from multi-join chain, got 0")
	}

	if !scannertest.HasSQLContaining(stmts, "JOIN orders ON 1=1") {
		t.Fatalf("expected first JOIN in synthetic SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "JOIN items ON 1=1") {
		t.Fatalf("expected second JOIN in synthetic SQL, got %+v", stmts)
	}
}

func TestGoquScannerNonGoFileIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	txtFile := filepath.Join(tmpDir, "queries.txt")
	if err := os.WriteFile(txtFile, []byte(`goqu.From("users").Select("*")`), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements for non-.go files, got %d", len(stmts))
	}
}

func TestGoquScannerParseErrorIsFatal(t *testing.T) {
	src := `package example
// this file has a syntax error
func queries( {
`

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "broken.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	_, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err == nil {
		t.Fatal("expected error for broken Go file")
	}
	if !strings.Contains(err.Error(), "parsing go file") {
		t.Fatalf("expected parse error, got: %v", err)
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
