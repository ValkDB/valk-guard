// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestMatchTable(t *testing.T) {
	t.Parallel()

	snap := schema.NewSnapshot()
	snap.ApplyCreateTable("users", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
	}, "migrations/001.sql", 1)
	snap.ApplyCreateTable("statuses", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
	}, "migrations/001.sql", 5)
	snap.ApplyCreateTable("address", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
	}, "migrations/001.sql", 10)

	tests := []struct {
		name       string
		modelTable string
		wantTable  string // empty means expect nil
	}{
		{
			name:       "exact match",
			modelTable: "users",
			wantTable:  "users",
		},
		{
			name:       "case insensitivity",
			modelTable: "Users",
			wantTable:  "users",
		},
		{
			name:       "case insensitivity mixed",
			modelTable: "USERS",
			wantTable:  "users",
		},
		{
			name:       "empty string returns nil",
			modelTable: "",
			wantTable:  "",
		},
		{
			name:       "no match returns nil",
			modelTable: "orders",
			wantTable:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := matchTable(snap, tt.modelTable)
			if tt.wantTable == "" {
				if got != nil {
					t.Fatalf("expected nil, got table %q", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected table %q, got nil", tt.wantTable)
			}
			if got.Name != tt.wantTable {
				t.Errorf("expected table name %q, got %q", tt.wantTable, got.Name)
			}
		})
	}
}
