// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import "testing"

func TestBuildModelSnapshotForSourceFiltersSourceAndColumns(t *testing.T) {
	t.Parallel()

	models := []ModelDef{
		{
			Table:  "users",
			Source: ModelSourceGo,
			Columns: []ModelColumn{
				{Name: "id", Type: "bigint"},
				{Name: " ", Type: "ignored"},
			},
			File: "models/user.go",
			Line: 12,
		},
		{
			Table:  "users",
			Source: ModelSourceSQLAlchemy,
			Columns: []ModelColumn{
				{Name: "email", Type: "text"},
			},
			File: "models.py",
			Line: 4,
		},
	}

	snap := BuildModelSnapshotForSource(models, ModelSourceGo)
	td := snap.Lookup("users")
	if td == nil {
		t.Fatal("expected users table in go snapshot")
	}
	if td.File != "models/user.go" {
		t.Fatalf("table file = %q, want %q", td.File, "models/user.go")
	}
	if len(td.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(td.Columns))
	}
	if _, ok := td.Columns["id"]; !ok {
		t.Fatal("expected id column in snapshot")
	}
	if _, ok := td.Columns["email"]; ok {
		t.Fatal("did not expect sqlalchemy column in go snapshot")
	}
}

func TestBuildModelSnapshotForSourceMergesDuplicateTables(t *testing.T) {
	t.Parallel()

	models := []ModelDef{
		{
			Table:  "users",
			Source: ModelSourceGo,
			Columns: []ModelColumn{
				{Name: "id"},
			},
			File: "a.go",
			Line: 1,
		},
		{
			Table:  "users",
			Source: ModelSourceGo,
			Columns: []ModelColumn{
				{Name: "email"},
			},
			File: "b.go",
			Line: 20,
		},
	}

	snap := BuildModelSnapshotForSource(models, ModelSourceGo)
	td := snap.Lookup("users")
	if td == nil {
		t.Fatal("expected users table")
	}
	if len(td.Columns) != 2 {
		t.Fatalf("expected merged columns, got %d", len(td.Columns))
	}
	if _, ok := td.Columns["id"]; !ok {
		t.Fatal("expected id column")
	}
	if _, ok := td.Columns["email"]; !ok {
		t.Fatal("expected email column")
	}
}
