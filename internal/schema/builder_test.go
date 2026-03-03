package schema

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestBuildFromStatements_CreateTable(t *testing.T) {
	stmts := []scanner.SQLStatement{{
		SQL: `CREATE TABLE users (
			id    SERIAL PRIMARY KEY,
			email TEXT NOT NULL,
			bio   TEXT DEFAULT ''
		)`,
		File: "001_users.sql",
		Line: 1,
	}}

	snap := BuildFromStatements(stmts)

	td, ok := snap.Tables["users"]
	if !ok {
		t.Fatal("expected users table in snapshot")
	}
	if td.File != "001_users.sql" {
		t.Errorf("file = %q, want %q", td.File, "001_users.sql")
	}
	if td.Line != 1 {
		t.Errorf("line = %d, want 1", td.Line)
	}

	wantCols := map[string]struct {
		typ        string
		nullable   bool
		hasDefault bool
	}{
		"id":    {typ: "serial", nullable: false, hasDefault: false},
		"email": {typ: "text", nullable: false, hasDefault: false},
		"bio":   {typ: "text", nullable: true, hasDefault: true},
	}

	for name, want := range wantCols {
		col, ok := td.Columns[name]
		if !ok {
			t.Errorf("column %q not found", name)
			continue
		}
		if col.Nullable != want.nullable {
			t.Errorf("column %q nullable = %v, want %v", name, col.Nullable, want.nullable)
		}
		if col.HasDefault != want.hasDefault {
			t.Errorf("column %q hasDefault = %v, want %v", name, col.HasDefault, want.hasDefault)
		}
	}
}

func TestBuildFromStatements_DropTable(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{SQL: "CREATE TABLE temp (id INTEGER)", File: "001.sql", Line: 1},
		{SQL: "DROP TABLE temp", File: "002.sql", Line: 1},
	}

	snap := BuildFromStatements(stmts)

	if _, ok := snap.Tables["temp"]; ok {
		t.Fatal("expected temp table to be dropped")
	}
}

func TestBuildFromStatements_DropColumn(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{
			SQL:  "CREATE TABLE users (id INTEGER, email TEXT, bio TEXT)",
			File: "001.sql", Line: 1,
		},
		{
			SQL:  "ALTER TABLE users DROP COLUMN bio",
			File: "002.sql", Line: 1,
		},
	}

	snap := BuildFromStatements(stmts)

	td := snap.Tables["users"]
	if td == nil {
		t.Fatal("expected users table to exist")
	}
	if _, ok := td.Columns["bio"]; ok {
		t.Fatal("expected bio column to be dropped")
	}
	if _, ok := td.Columns["id"]; !ok {
		t.Fatal("expected id column to remain")
	}
	if _, ok := td.Columns["email"]; !ok {
		t.Fatal("expected email column to remain")
	}
}

func TestBuildFromStatements_AlterTableAddColumn(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{
			SQL:  "CREATE TABLE users (id INTEGER)",
			File: "001.sql", Line: 1,
		},
		{
			SQL:  "ALTER TABLE users ADD COLUMN email TEXT NOT NULL",
			File: "002.sql", Line: 1,
		},
	}

	snap := BuildFromStatements(stmts)

	td := snap.Tables["users"]
	if td == nil {
		t.Fatal("expected users table to exist")
	}
	if _, ok := td.Columns["email"]; !ok {
		t.Fatal("expected email column to be added")
	}
}

func TestBuildFromStatements_CreateThenDropColumn(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{
			SQL:  "CREATE TABLE orders (id INTEGER, amount NUMERIC, notes TEXT)",
			File: "001.sql", Line: 1,
		},
		{
			SQL:  "ALTER TABLE orders DROP COLUMN notes",
			File: "002.sql", Line: 5,
		},
	}

	snap := BuildFromStatements(stmts)

	td := snap.Tables["orders"]
	if td == nil {
		t.Fatal("expected orders table")
	}
	if _, ok := td.Columns["notes"]; ok {
		t.Fatal("expected notes column to be dropped")
	}
	if len(td.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(td.Columns))
	}
}

func TestBuildFromStatements_EmptyAndNonDDL(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{SQL: "", File: "empty.sql", Line: 1},
		{SQL: "SELECT 1", File: "query.sql", Line: 1},
		{SQL: "INSERT INTO users VALUES (1)", File: "insert.sql", Line: 1},
	}

	snap := BuildFromStatements(stmts)

	if len(snap.Tables) != 0 {
		t.Fatalf("expected empty snapshot, got %d tables", len(snap.Tables))
	}
}

func TestBuildFromStatements_NilInput(t *testing.T) {
	snap := BuildFromStatements(nil)
	if len(snap.Tables) != 0 {
		t.Fatal("expected empty snapshot for nil input")
	}
}

func TestBuildFromStatements_InvalidSQL(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{SQL: "NOT VALID SQL !!!", File: "bad.sql", Line: 1},
	}

	snap := BuildFromStatements(stmts)

	if len(snap.Tables) != 0 {
		t.Fatal("expected empty snapshot for invalid SQL")
	}
}

func TestBuildFromStatements_CreateTableIfNotExists(t *testing.T) {
	stmts := []scanner.SQLStatement{{
		SQL: `CREATE TABLE IF NOT EXISTS products (
			id         BIGSERIAL PRIMARY KEY,
			name       VARCHAR(255) NOT NULL,
			price      NUMERIC(10,2) DEFAULT 0.00,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		File: "003.sql",
		Line: 1,
	}}

	snap := BuildFromStatements(stmts)

	td, ok := snap.Tables["products"]
	if !ok {
		t.Fatal("expected products table")
	}
	if len(td.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(td.Columns))
	}

	// Verify the created_at column has both NOT NULL and a default.
	col, ok := td.Columns["created_at"]
	if !ok {
		t.Fatal("expected created_at column")
	}
	if col.Nullable {
		t.Error("expected created_at NOT NULL")
	}
	if !col.HasDefault {
		t.Error("expected created_at to have a default")
	}
}

func TestBuildFromStatements_MultipleTablesSequence(t *testing.T) {
	stmts := []scanner.SQLStatement{
		{SQL: "CREATE TABLE a (id INTEGER)", File: "001.sql", Line: 1},
		{SQL: "CREATE TABLE b (id INTEGER, ref INTEGER)", File: "001.sql", Line: 3},
		{SQL: "ALTER TABLE b ADD COLUMN name TEXT", File: "002.sql", Line: 1},
		{SQL: "ALTER TABLE a DROP COLUMN id", File: "003.sql", Line: 1},
	}

	snap := BuildFromStatements(stmts)

	if len(snap.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(snap.Tables))
	}

	tdA := snap.Tables["a"]
	if len(tdA.Columns) != 0 {
		t.Errorf("table a: expected 0 columns after drop, got %d", len(tdA.Columns))
	}

	tdB := snap.Tables["b"]
	if len(tdB.Columns) != 3 {
		t.Errorf("table b: expected 3 columns, got %d", len(tdB.Columns))
	}
}
