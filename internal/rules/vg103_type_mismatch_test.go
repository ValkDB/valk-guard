// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestTypeMismatchRule(t *testing.T) {
	t.Parallel()

	rule := &TypeMismatchRule{}

	if rule.ID() != "VG103" {
		t.Fatalf("ID() = %q, want VG103", rule.ID())
	}
	if rule.DefaultSeverity() != SeverityWarning {
		t.Fatalf("DefaultSeverity() = %q, want %q", rule.DefaultSeverity(), SeverityWarning)
	}

	tests := []struct {
		name      string
		snap      *schema.Snapshot
		models    []schema.ModelDef
		wantCount int
		wantMsg   string
		wantLine  int // if non-zero, assert finding line number
	}{
		{
			name: "compatible types",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "varchar(255)"},
					{Name: "active", Type: "boolean"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "integer"},
					{Name: "email", Field: "Email", Type: "string"},
					{Name: "active", Field: "Active", Type: "boolean"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "python parameterized type is normalized",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "email", Type: "varchar(255)"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "email", Field: "Email", Type: "String(255)"},
				},
				File: "models/user.py",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "type mismatch detected",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "text"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "integer"},
					{Name: "email", Field: "Email", Type: "integer"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `column "email" type mismatch: model has "integer" but migration has "text"; align model type or migration column type`,
		},
		{
			name: "interval is not integer-compatible",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "duration", Type: "interval"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "duration", Field: "Duration", Type: "int64"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `column "duration" type mismatch: model has "int64" but migration has "interval"; align model type or migration column type`,
		},
		{
			name: "empty model type skipped",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
					{Name: "email", Type: "text"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: ""},
					{Name: "email", Field: "Email", Type: ""},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "unknown model type skipped",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "CustomType"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "bigint compatible with int",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "bigint"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "int64"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "timestamp compatible with datetime",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "created_at", Type: "timestamptz"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "created_at", Field: "CreatedAt", Type: "datetime"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "column not in schema skipped",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "missing", Field: "Missing", Type: "integer"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "table not matched skipped",
			snap: schema.NewSnapshot(),
			models: []schema.ModelDef{{
				Table: "orders",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "integer"},
				},
				File: "models/order.go",
				Line: 5,
			}},
			wantCount: 0,
		},
		{
			name: "sql.NullInt64 compatible with integer",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "age", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "age", Field: "Age", Type: "sql.NullInt64", Line: 12},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "pointer string compatible with text",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "bio", Type: "text"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "bio", Field: "Bio", Type: "*string", Line: 14},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "sql.NullBool compatible with boolean",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "verified", Type: "boolean"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "verified", Field: "Verified", Type: "sql.NullBool", Line: 16},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "sql.NullFloat64 compatible with numeric",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "balance", Type: "numeric"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "balance", Field: "Balance", Type: "sql.NullFloat64", Line: 18},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "sql.NullString compatible with varchar",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "name", Type: "varchar(100)"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "name", Field: "Name", Type: "sql.NullString", Line: 20},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "sql.NullTime compatible with timestamptz",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "deleted_at", Type: "timestamptz"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "deleted_at", Field: "DeletedAt", Type: "sql.NullTime", Line: 22},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "finding uses column line not model line",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "email", Type: "text"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "email", Field: "Email", Type: "integer", Line: 25},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantLine:  25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			findings := rule.CheckSchema(context.Background(), tt.snap, tt.models)
			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
			if tt.wantMsg != "" && len(findings) > 0 && findings[0].Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", findings[0].Message, tt.wantMsg)
			}
			if tt.wantLine != 0 && len(findings) > 0 && findings[0].Line != tt.wantLine {
				t.Errorf("Line = %d, want %d", findings[0].Line, tt.wantLine)
			}
			for _, f := range findings {
				if f.RuleID != "VG103" {
					t.Errorf("RuleID = %q, want VG103", f.RuleID)
				}
				if f.SQL != "" {
					t.Errorf("SQL should be empty for schema rules, got %q", f.SQL)
				}
			}
		})
	}
}
