package sqlalchemy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestSQLAlchemyScannerExtractsRawAndSyntheticSQL(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "queries.py")

	content := `from sqlalchemy import text

def run(session, User, Address):
    session.execute(text("SELECT * FROM users"))
    session.query(User).join(Address).all()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &SQLAlchemyScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) < 2 {
		t.Fatalf("expected at least 2 statements, got %d: %+v", len(stmts), stmts)
	}
	if !hasSQL(stmts, "SELECT * FROM users") {
		t.Fatalf("expected raw execute(text(...)) SQL to be extracted")
	}
	if !hasSQLContaining(stmts, `/* valk-guard:synthetic sqlalchemy-ast */ SELECT * FROM "User" JOIN "Address" ON 1=1`) {
		t.Fatalf("expected synthetic SQL with JOIN from query builder, got %+v", stmts)
	}
}

func TestSQLAlchemyScannerBuilderChainsTriggerRules(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "builder.py")

	content := `from sqlalchemy import select

def run(session, User, Address, Roles):
    session.query(User).join(Address).all()
    session.query(User).join(Address).filter(Address.street.like("%Main%")).all()
    session.query(User).join(Roles).delete()
    session.query(User).join(Roles).update({"active": False})
    select(User).join(Address)
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &SQLAlchemyScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	findingsByRule := collectFindingsByRule(t, stmts)
	requiredRules := []string{"VG001", "VG002", "VG003", "VG004", "VG005"}
	for _, ruleID := range requiredRules {
		if findingsByRule[ruleID] == 0 {
			t.Fatalf("expected %s finding from SQLAlchemy builder chains, got none (all findings: %+v)", ruleID, findingsByRule)
		}
	}

	if !hasSQLContaining(stmts, `JOIN "Address" ON 1=1`) {
		t.Fatalf("expected JOIN to be preserved in synthetic SQL, got %+v", stmts)
	}
	if !hasSQLContaining(stmts, `"Address"."street" LIKE '%Main%'`) {
		t.Fatalf("expected LIKE predicate to be preserved in synthetic SQL, got %+v", stmts)
	}
}

func TestSQLAlchemyScannerDirectiveSuppressionOnSyntheticSQL(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "directive.py")

	content := `def run(session, User, Roles):
    # valk-guard:disable VG003
    session.query(User).join(Roles).delete()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &SQLAlchemyScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %+v", len(stmts), stmts)
	}
	if len(stmts[0].Disabled) != 1 || stmts[0].Disabled[0] != "VG003" {
		t.Fatalf("expected disabled=[VG003], got %v", stmts[0].Disabled)
	}
	if !strings.HasPrefix(stmts[0].SQL, "/* valk-guard:synthetic sqlalchemy-ast */") {
		t.Fatalf("expected synthetic marker prefix, got %q", stmts[0].SQL)
	}
}

func TestSQLAlchemyScannerSkipsNonPython(t *testing.T) {
	tmpDir := t.TempDir()
	txtFile := filepath.Join(tmpDir, "not_python.txt")
	if err := os.WriteFile(txtFile, []byte(`session.query(User).all()`), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &SQLAlchemyScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements for non-.py files, got %d", len(stmts))
	}
}

func TestSQLAlchemyScannerSkipsFilesWithoutKeywords(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "hello.py")
	if err := os.WriteFile(pyFile, []byte(`print("hello world")`), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &SQLAlchemyScanner{}
	stmts, err := s.Scan([]string{tmpDir})
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
