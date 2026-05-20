// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	scannercore "github.com/valkdb/valk-guard/internal/scanner"
	csharpscanner "github.com/valkdb/valk-guard/internal/scanner/csharp"
	goquscanner "github.com/valkdb/valk-guard/internal/scanner/goqu"
	sqlalchemyscanner "github.com/valkdb/valk-guard/internal/scanner/sqlalchemy"
	"github.com/valkdb/valk-guard/internal/scannertest"
)

func TestORMScannerParityComplexRuleTriggers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		ruleID    string
		goSource  string
		pySource  string
		csSource  string
		fragments []string
	}{
		{
			name:   "select star with join",
			ruleID: "VG001",
			goSource: `package parity

import goqu "github.com/doug-martin/goqu/v9"

func run() {
	goqu.From("Users").Join(goqu.T("Addresses"), goqu.On(goqu.Ex{"Users.Id": goqu.I("Addresses.UserId")})).Select(goqu.Star())
}
`,
			pySource: `def run(session, User, Address):
    session.query(User).join(Address).all()
`,
			csSource: `class Repo {
    void Run(MyDbContext db) {
        db.Users.Join(db.Addresses, u => u.Id, a => a.UserId, (u, a) => u).ToList();
    }
}
`,
			fragments: []string{"SELECT *", "JOIN"},
		},
		{
			name:   "leading wildcard like with join",
			ruleID: "VG005",
			goSource: `package parity

import goqu "github.com/doug-martin/goqu/v9"

func run() {
	goqu.From("Users").Join(goqu.T("Addresses"), goqu.On(goqu.Ex{"Users.Id": goqu.I("Addresses.UserId")})).Where(goqu.C("Email").Like("%@gmail.com")).Select("Users.Id").Limit(10)
}
`,
			pySource: `def run(session, User, Address):
    session.query(User.id).join(Address).filter(Address.street.like("%Main%")).limit(10).all()
`,
			csSource: `class Repo {
    void Run(MyDbContext db) {
        db.Users.Join(db.Addresses, u => u.Id, a => a.UserId, (u, a) => u).Where(u => u.Email.Contains("@gmail.com")).Take(10).ToList();
    }
}
`,
			fragments: []string{"JOIN", "LIKE"},
		},
		{
			name:   "unbounded ordered joined select",
			ruleID: "VG004",
			goSource: `package parity

import goqu "github.com/doug-martin/goqu/v9"

func run() {
	goqu.From("Logs").LeftJoin(goqu.T("Users"), goqu.On(goqu.Ex{"Logs.UserId": goqu.I("Users.Id")})).Select("Logs.Id", "Logs.Message").Order(goqu.I("Logs.Id").Desc())
}
`,
			pySource: `def run(session, Log, User):
    session.query(Log.id, Log.message).join(User).order_by(Log.id.desc()).all()
`,
			csSource: `class Repo {
    void Run(MyDbContext db) {
        db.Logs.Join(db.Users, l => l.UserId, u => u.Id, (l, u) => l).OrderByDescending(l => l.Id).Select(l => new { l.Id, l.Message }).ToList();
    }
}
`,
			fragments: []string{"JOIN", "ORDER BY"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			goSQL := scanGoquParity(t, tc.goSource)
			pySQL := scanSQLAlchemyParity(t, tc.pySource)
			csSQL := scanCSharpParity(t, tc.csSource)

			assertRuleFires(t, "goqu", tc.ruleID, goSQL)
			assertRuleFires(t, "sqlalchemy", tc.ruleID, pySQL)
			assertRuleFires(t, "csharp", tc.ruleID, csSQL)

			for _, fragment := range tc.fragments {
				assertAnySQLContains(t, "goqu", goSQL, fragment)
				assertAnySQLContains(t, "sqlalchemy", pySQL, fragment)
				assertAnySQLContains(t, "csharp", csSQL, fragment)
			}
		})
	}
}

func scanGoquParity(t *testing.T, src string) []scannercore.SQLStatement {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "query.go"), src)
	stmts, err := scannercore.Collect((&goquscanner.Scanner{}).Scan(context.Background(), []string{dir}))
	if err != nil {
		t.Fatalf("goqu scan error: %v", err)
	}
	return stmts
}

func scanSQLAlchemyParity(t *testing.T, src string) []scannercore.SQLStatement {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "query.py"), src)
	stmts, err := scannercore.Collect((&sqlalchemyscanner.Scanner{}).Scan(context.Background(), []string{dir}))
	if err != nil {
		t.Fatalf("sqlalchemy scan error: %v", err)
	}
	return stmts
}

func scanCSharpParity(t *testing.T, src string) []scannercore.SQLStatement {
	t.Helper()
	requireDotnetForParity(t)
	dir := t.TempDir()
	if os.Getenv("VALK_DOTNET_PATH") != "" {
		var err error
		dir, err = os.MkdirTemp(".", ".csharp-parity-*")
		if err != nil {
			t.Fatalf("create local C# temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	writeFile(t, filepath.Join(dir, "Query.cs"), src)
	project := filepath.Join("csharp", "roslynextractor", "RoslynExtractor.csproj")
	s := &csharpscanner.Scanner{DotnetPath: os.Getenv("VALK_DOTNET_PATH"), ProjectPath: project}
	stmts, err := scannercore.Collect(s.Scan(context.Background(), []string{dir}))
	if err != nil {
		t.Fatalf("csharp scan error: %v", err)
	}
	return stmts
}

func requireDotnetForParity(t *testing.T) {
	t.Helper()
	if path := os.Getenv("VALK_DOTNET_PATH"); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("VALK_DOTNET_PATH is set but not usable: %v", err)
		}
		return
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		if os.Getenv("VALK_REQUIRE_DOTNET") == "1" {
			t.Fatalf("dotnet SDK is required for C# parity tests: %v", err)
		}
		t.Skip("dotnet SDK is required for C# parity tests")
	}
}

func writeFile(t *testing.T, path, src string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertRuleFires(t *testing.T, engine, ruleID string, stmts []scannercore.SQLStatement) {
	t.Helper()
	findings := scannertest.CollectFindingsByRule(t, stmts)
	if findings[ruleID] == 0 {
		t.Fatalf("expected %s to fire %s, got findings=%+v sql=%s", engine, ruleID, findings, joinSQL(stmts))
	}
}

func assertAnySQLContains(t *testing.T, engine string, stmts []scannercore.SQLStatement, fragment string) {
	t.Helper()
	needle := strings.ToUpper(fragment)
	for _, stmt := range stmts {
		if strings.Contains(strings.ToUpper(stmt.SQL), needle) {
			return
		}
	}
	t.Fatalf("expected %s SQL to contain %q, got %s", engine, fragment, joinSQL(stmts))
}

func joinSQL(stmts []scannercore.SQLStatement) string {
	parts := make([]string, 0, len(stmts))
	for _, stmt := range stmts {
		parts = append(parts, stmt.SQL)
	}
	return strings.Join(parts, " | ")
}
