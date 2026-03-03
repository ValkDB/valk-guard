// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import "testing"

func TestApplyCreateTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		table    string
		columns  []ColumnDef
		file     string
		line     int
		wantCols int
	}{
		{
			name:  "single column",
			table: "users",
			columns: []ColumnDef{
				{Name: "id", Type: "integer", Nullable: false, HasDefault: true},
			},
			file:     "001.sql",
			line:     1,
			wantCols: 1,
		},
		{
			name:  "multiple columns",
			table: "orders",
			columns: []ColumnDef{
				{Name: "id", Type: "bigint"},
				{Name: "user_id", Type: "integer"},
				{Name: "total", Type: "numeric(10,2)", Nullable: true},
			},
			file:     "002.sql",
			line:     5,
			wantCols: 3,
		},
		{
			name:     "empty columns",
			table:    "empty_table",
			columns:  nil,
			file:     "003.sql",
			line:     1,
			wantCols: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snap := NewSnapshot()
			snap.ApplyCreateTable(tt.table, tt.columns, tt.file, tt.line)

			td, ok := snap.Tables[tt.table]
			if !ok {
				t.Fatalf("table %q not found in snapshot", tt.table)
			}
			if td.Name != tt.table {
				t.Errorf("table name = %q, want %q", td.Name, tt.table)
			}
			if td.File != tt.file {
				t.Errorf("file = %q, want %q", td.File, tt.file)
			}
			if td.Line != tt.line {
				t.Errorf("line = %d, want %d", td.Line, tt.line)
			}
			if len(td.Columns) != tt.wantCols {
				t.Fatalf("column count = %d, want %d", len(td.Columns), tt.wantCols)
			}
		})
	}
}

func TestApplyCreateTableOverwrites(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("users", []ColumnDef{{Name: "id", Type: "integer"}}, "v1.sql", 1)
	snap.ApplyCreateTable("users", []ColumnDef{{Name: "id", Type: "bigint"}, {Name: "email", Type: "text"}}, "v2.sql", 10)

	td := snap.Tables["users"]
	if td.File != "v2.sql" {
		t.Errorf("expected last writer wins, got file %q", td.File)
	}
	if len(td.Columns) != 2 {
		t.Errorf("expected 2 columns after overwrite, got %d", len(td.Columns))
	}
}

func TestApplyCreateTableCaseInsensitive(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("Users", []ColumnDef{{Name: "ID", Type: "integer"}}, "a.sql", 1)

	if _, ok := snap.Tables["users"]; !ok {
		t.Fatal("expected case-insensitive table lookup")
	}
	td := snap.Tables["users"]
	if _, ok := td.Columns["id"]; !ok {
		t.Fatal("expected case-insensitive column lookup")
	}
}

func TestApplyDropTable(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("users", []ColumnDef{{Name: "id", Type: "integer"}}, "a.sql", 1)
	snap.ApplyDropTable("users")

	if _, ok := snap.Tables["users"]; ok {
		t.Fatal("expected table to be removed")
	}
}

func TestApplyDropTableNonExistent(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	// Should not panic on dropping a table that doesn't exist.
	snap.ApplyDropTable("nonexistent")
	if len(snap.Tables) != 0 {
		t.Fatal("expected empty snapshot")
	}
}

func TestApplyDropColumn(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("users", []ColumnDef{
		{Name: "id", Type: "integer"},
		{Name: "email", Type: "text"},
	}, "a.sql", 1)

	snap.ApplyDropColumn("users", "email")

	td := snap.Tables["users"]
	if _, ok := td.Columns["email"]; ok {
		t.Fatal("expected column email to be removed")
	}
	if _, ok := td.Columns["id"]; !ok {
		t.Fatal("expected column id to remain")
	}
}

func TestApplyDropColumnNonExistentTable(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	// Should not panic.
	snap.ApplyDropColumn("nonexistent", "col")
}

func TestApplyDropColumnNonExistentColumn(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("users", []ColumnDef{{Name: "id", Type: "integer"}}, "a.sql", 1)
	// Should not panic.
	snap.ApplyDropColumn("users", "nonexistent")
	if len(snap.Tables["users"].Columns) != 1 {
		t.Fatal("expected original column to remain")
	}
}

func TestApplyAddColumn(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	snap.ApplyCreateTable("users", []ColumnDef{{Name: "id", Type: "integer"}}, "a.sql", 1)

	snap.ApplyAddColumn("users", ColumnDef{Name: "email", Type: "text", Nullable: true})

	td := snap.Tables["users"]
	if len(td.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(td.Columns))
	}
	col, ok := td.Columns["email"]
	if !ok {
		t.Fatal("expected email column to be present")
	}
	if !col.Nullable {
		t.Error("expected email to be nullable")
	}
}

func TestApplyAddColumnNonExistentTable(t *testing.T) {
	t.Parallel()

	snap := NewSnapshot()
	// Should not panic; no-op for missing tables.
	snap.ApplyAddColumn("nonexistent", ColumnDef{Name: "col", Type: "text"})
	if len(snap.Tables) != 0 {
		t.Fatal("expected empty snapshot")
	}
}
