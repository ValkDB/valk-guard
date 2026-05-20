// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package sqlalchemy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scannertest"
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

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) < 2 {
		t.Fatalf("expected at least 2 statements, got %d: %+v", len(stmts), stmts)
	}
	if !scannertest.HasSQL(stmts, "SELECT * FROM users") {
		t.Fatalf("expected raw execute(text(...)) SQL to be extracted")
	}
	if !scannertest.HasSQLContaining(stmts, `/* valk-guard:synthetic sqlalchemy-ast */ SELECT * FROM "User" JOIN "Address" ON 1=1`) {
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
    session.query(User).with_for_update().all()
    session.query(User).join(Roles).delete()
    session.query(User).join(Roles).update({"active": False})
    select(User).join(Address)
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
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
			t.Fatalf("expected %s finding from SQLAlchemy builder chains, got none (all findings: %+v)", ruleID, findingsByRule)
		}
	}

	if !scannertest.HasSQLContaining(stmts, `JOIN "Address" ON 1=1`) {
		t.Fatalf("expected JOIN to be preserved in synthetic SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, `"Address"."street" LIKE '%Main%'`) {
		t.Fatalf("expected LIKE predicate to be preserved in synthetic SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "FOR UPDATE") {
		t.Fatalf("expected FOR UPDATE to be preserved in synthetic SQL, got %+v", stmts)
	}
}

func TestSQLAlchemyScannerDirectiveSuppressionOnSyntheticSQL(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "directive.py")

	content := `from sqlalchemy.orm import Session

def run(session, User, Roles):
    # valk-guard:disable VG003
    session.query(User).join(Roles).delete()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
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
	if len(stmts[0].Disabled) != 1 || stmts[0].Disabled[0] != "VG003" {
		t.Fatalf("expected disabled=[VG003], got %v", stmts[0].Disabled)
	}
	if !strings.HasPrefix(stmts[0].SQL, "/* valk-guard:synthetic sqlalchemy-ast */") {
		t.Fatalf("expected synthetic marker prefix, got %q", stmts[0].SQL)
	}
}

func TestSQLAlchemyScannerResolvesTablename(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "models.py")

	content := `from sqlalchemy import create_engine, select
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column, sessionmaker
from sqlalchemy import String, Boolean, ForeignKey

class Base(DeclarativeBase):
    pass

class User(Base):
    __tablename__ = "users"
    id: Mapped[int] = mapped_column(primary_key=True)
    email: Mapped[str] = mapped_column(String(255))
    active: Mapped[bool] = mapped_column(Boolean)

class Order(Base):
    __tablename__ = "orders"
    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id"))

engine = create_engine("postgresql://localhost/example")
session = sessionmaker(bind=engine)()

session.query(User.id, User.email).filter(User.active == True).limit(10).all()
session.query(User).join(Order, Order.user_id == User.id).filter(User.active == True).limit(50).all()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	// Verify __tablename__ is used instead of class name
	for _, stmt := range stmts {
		if !strings.Contains(stmt.SQL, "valk-guard:synthetic") {
			continue
		}
		if strings.Contains(stmt.SQL, `"User"`) {
			t.Errorf("synthetic SQL should use __tablename__ 'users' not class name 'User': %s", stmt.SQL)
		}
		if strings.Contains(stmt.SQL, `"Order"`) {
			t.Errorf("synthetic SQL should use __tablename__ 'orders' not class name 'Order': %s", stmt.SQL)
		}
	}

	if !scannertest.HasSQLContaining(stmts, `"users"."id"`) {
		t.Errorf("expected resolved table name 'users' in column references, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, `FROM "users"`) {
		t.Errorf("expected resolved table name 'users' in FROM clause, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, `JOIN "orders" ON "orders"."user_id" = "users"."id"`) {
		t.Errorf("expected resolved table name 'orders' in JOIN clause, got %+v", stmts)
	}
}

func TestSQLAlchemyScannerSkipsNonPython(t *testing.T) {
	tmpDir := t.TempDir()
	txtFile := filepath.Join(tmpDir, "not_python.txt")
	if err := os.WriteFile(txtFile, []byte(`session.query(User).all()`), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
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

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}
}

func TestSQLAlchemyScannerExtractsSyntheticSQLWithoutDirectSQLAlchemyImport(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "service.py")

	content := `def run(session, User):
    session.query(User).filter(User.id == 1).all()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 synthetic statement, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0].SQL, "valk-guard:synthetic sqlalchemy-ast") {
		t.Fatalf("expected synthetic SQL marker, got %q", stmts[0].SQL)
	}
}

func TestSQLAlchemyScannerResolvesImportedModelTablenames(t *testing.T) {
	tmpDir := t.TempDir()
	modelsFile := filepath.Join(tmpDir, "models.py")
	serviceFile := filepath.Join(tmpDir, "service.py")

	models := `from sqlalchemy import Boolean, Integer, String
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

class Base(DeclarativeBase):
    pass

class User(Base):
    __tablename__ = "users"
    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    email: Mapped[str] = mapped_column(String(255))
    active: Mapped[bool] = mapped_column(Boolean)

class Order(Base):
    __tablename__ = "orders"
    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[int] = mapped_column(Integer)
    status: Mapped[str] = mapped_column(String(50))
`
	service := `from sqlalchemy.orm import Session
from models import Order, User

def run(session: Session):
    return (
        session.query(User.id, User.email, Order.status)
        .outerjoin(Order, Order.user_id == User.id)
        .filter(User.active.is_(True))
        .limit(25)
        .all()
    )
`

	if err := os.WriteFile(modelsFile, []byte(models), 0644); err != nil {
		t.Fatalf("failed to write models file: %v", err)
	}
	if err := os.WriteFile(serviceFile, []byte(service), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) == 0 {
		t.Fatal("expected synthetic SQL from imported SQLAlchemy models, got 0 statements")
	}
	if !scannertest.HasSQLContaining(stmts, `FROM "users" LEFT JOIN "orders" ON "orders"."user_id" = "users"."id"`) {
		t.Fatalf("expected imported __tablename__ values in JOIN SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, `"orders"."status"`) {
		t.Fatalf("expected imported __tablename__ values in projection SQL, got %+v", stmts)
	}
}

func TestSQLAlchemyScannerErrorsOnSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "broken.py")
	content := `import sqlalchemy
def run(session)
    session.execute("SELECT 1")
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	_, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err == nil {
		t.Fatal("expected syntax error, got nil")
	}
	if !strings.Contains(err.Error(), "python script execution failed") {
		t.Fatalf("expected extractor failure error, got %v", err)
	}
}

func TestSQLAlchemyScannerParityFeatureClauses(t *testing.T) {
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "parity.py")

	content := `from sqlalchemy import func

def run(session, User, Address, min_count):
    session.query(User.id, func.count(Address.id)).distinct().join(Address, Address.user_id == User.id).filter(User.id.in_([1, 2, 3])).group_by(User.id).having(func.count(Address.id) > min_count).order_by(User.email.desc(), User.id).limit(10).offset(20).all()
    session.query(User.id).filter(User.id.notin_([1, 2, 3])).limit(1).all()
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	for _, want := range []string{
		`SELECT DISTINCT "User"."id", COUNT("Address"."id") FROM "User"`,
		`JOIN "Address" ON "Address"."user_id" = "User"."id"`,
		`"User"."id" IN ($1, $2, $3)`,
		`GROUP BY "User"."id" HAVING COUNT("Address"."id") > $4`,
		`ORDER BY "User"."email" DESC, "User"."id" ASC LIMIT 10 OFFSET 20`,
		`"User"."id" NOT IN ($1, $2, $3)`,
	} {
		if !scannertest.HasSQLContaining(stmts, want) {
			t.Fatalf("expected synthetic SQL containing %q, got %+v", want, stmts)
		}
	}
}
