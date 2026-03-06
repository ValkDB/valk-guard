// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestTableNotFoundRule(t *testing.T) {
	t.Parallel()

	rule := &TableNotFoundRule{}

	if rule.ID() != "VG104" {
		t.Fatalf("ID() = %q, want VG104", rule.ID())
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
			name: "table exists exact match",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table:         "users",
				TableExplicit: true,
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "table name mismatch reports finding",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table:         "user",
				TableExplicit: true,
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 1,
			wantMsg:   `table "user" referenced by model has no CREATE TABLE in migrations; add migration DDL or fix TableName()`,
		},
		{
			name: "inferred table names are skipped",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table:         "configrow",
				TableExplicit: false,
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/config.go",
				Line: 3,
			}},
			wantCount: 0,
		},
		{
			name: "table not found",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{{
				Table:         "orders",
				TableExplicit: true,
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/order.go",
				Line: 5,
			}},
			wantCount: 1,
			wantMsg:   `table "orders" referenced by model has no CREATE TABLE in migrations; add migration DDL or fix TableName()`,
		},
		{
			name: "empty snapshot skipped",
			snap: schema.NewSnapshot(),
			models: []schema.ModelDef{{
				Table:         "users",
				TableExplicit: true,
				Columns: []schema.ModelColumn{
					{Name: "id", Field: "ID"},
				},
				File: "models/user.go",
				Line: 10,
			}},
			wantCount: 0,
		},
		{
			name: "multiple models some missing",
			snap: func() *schema.Snapshot {
				s := schema.NewSnapshot()
				s.ApplyCreateTable("users", []schema.ColumnDef{
					{Name: "id", Type: "integer"},
				}, "migrations/001.sql", 1)
				return s
			}(),
			models: []schema.ModelDef{
				{
					Table:         "users",
					TableExplicit: true,
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID"},
					},
					File: "models/user.go",
					Line: 10,
				},
				{
					Table:         "orders",
					TableExplicit: true,
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID"},
					},
					File: "models/order.go",
					Line: 5,
				},
			},
			wantCount: 1,
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
				if f.RuleID != "VG104" {
					t.Errorf("RuleID = %q, want VG104", f.RuleID)
				}
				if f.SQL != "" {
					t.Errorf("SQL should be empty for schema rules, got %q", f.SQL)
				}
			}
		})
	}
}
