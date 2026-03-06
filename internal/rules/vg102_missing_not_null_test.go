// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestMissingNotNullRule(t *testing.T) {
	t.Parallel()

	rule := &MissingNotNullRule{}

	if rule.ID() != "VG102" {
		t.Fatalf("ID() = %q, want VG102", rule.ID())
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
			name: "all not-null columns defined",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer", Nullable: false, HasDefault: false},
					{Name: "email", Type: "text", Nullable: false, HasDefault: false},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
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
			name: "missing not-null column",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer", Nullable: false, HasDefault: false},
					{Name: "email", Type: "text", Nullable: false, HasDefault: false},
					{Name: "username", Type: "text", Nullable: false, HasDefault: false},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
					{Name: "email", Field: "Email"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `table "users" has NOT NULL column "username" but model "users" does not define it; add the field or make the migration column nullable/defaulted`,
		},
		{
			name: "nullable column not flagged",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer", Nullable: false, HasDefault: false},
					{Name: "bio", Type: "text", Nullable: true, HasDefault: false},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "not-null with default not flagged",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer", Nullable: false, HasDefault: false},
					{Name: "status", Type: "text", Nullable: false, HasDefault: true},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table: "users",
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
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
					{Name: "id", Field: "ID"},
				},
				File: "models/order.go",
				Line: 5,
			}},
			wantCount: 0,
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
			for _, f := range findings {
				if f.RuleID != "VG102" {
					t.Errorf("RuleID = %q, want VG102", f.RuleID)
				}
				if f.SQL != "" {
					t.Errorf("SQL should be empty for schema rules, got %q", f.SQL)
				}
			}
		})
	}
}
