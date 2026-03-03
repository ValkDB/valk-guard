package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestTypeMismatchRule(t *testing.T) {
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
				Table: "user",
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
				Table: "user",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID", Type: "integer"},
					{Name: "email", Field: "Email", Type: "integer"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `column "email" type mismatch: model has "integer" but migration has "text"`,
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
				Table: "user",
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
				Table: "user",
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
				Table: "user",
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
				Table: "user",
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
				Table: "user",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := rule.CheckSchema(tt.snap, tt.models)
			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
			if tt.wantMsg != "" && len(findings) > 0 && findings[0].Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", findings[0].Message, tt.wantMsg)
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
