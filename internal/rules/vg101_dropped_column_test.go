package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestDroppedColumnRule(t *testing.T) {
	rule := &DroppedColumnRule{}

	if rule.ID() != "VG101" {
		t.Fatalf("ID() = %q, want VG101", rule.ID())
	}
	if rule.DefaultSeverity() != SeverityError {
		t.Fatalf("DefaultSeverity() = %q, want %q", rule.DefaultSeverity(), SeverityError)
	}

	tests := []struct {
		name      string
		snap      *schema.Snapshot
		models    []schema.ModelDef
		wantCount int
		wantMsg   string
	}{
		{
			name: "all columns exist",
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
					{Name: "id", Field: "ID"},
					{Name: "email", Field: "Email"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "dropped column detected",
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
					{Name: "id", Field: "ID"},
					{Name: "email", Field: "Email"},
					{Name: "legacy_field", Field: "LegacyField"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `model "user" references column "legacy_field" not found in table "users" schema`,
		},
		{
			name: "table not in snapshot skipped",
			snap: schema.NewSnapshot(),
			models: []schema.ModelDef{{
				Table: "orders",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/order.go",
				Line: 5,
			}},
			wantCount: 0,
		},
		{
			name: "multiple dropped columns",
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
					{Name: "id", Field: "ID"},
					{Name: "name", Field: "Name"},
					{Name: "age", Field: "Age"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 2,
		},
		{
			name: "case insensitive column match",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "ID", Type: "integer"},
					{Name: "Email", Type: "text"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "user",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
					{Name: "email", Field: "Email"},
				},
				File: "models/user.go",
				Line: 10,
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
				if f.RuleID != "VG101" {
					t.Errorf("RuleID = %q, want VG101", f.RuleID)
				}
				if f.SQL != "" {
					t.Errorf("SQL should be empty for schema rules, got %q", f.SQL)
				}
			}
		})
	}
}
