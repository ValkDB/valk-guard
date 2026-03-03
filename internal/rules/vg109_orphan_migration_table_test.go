// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestOrphanMigrationTableRule(t *testing.T) {
	t.Parallel()

	rule := &OrphanMigrationTableRule{}
	snap := schema.NewSnapshot()
	snap.ApplyCreateTable("users", []schema.ColumnDef{{Name: "id", Type: "integer"}}, "migrations/001.sql", 1)
	snap.ApplyCreateTable("orders", []schema.ColumnDef{{Name: "id", Type: "integer"}}, "migrations/001.sql", 8)

	models := []schema.ModelDef{
		{Table: "users", Source: schema.ModelSourceGo, File: "go/models.go", Line: 3},
	}

	findings := rule.CheckSchema(snap, models)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].Message != `migration table "orders" has no matching model mapping` {
		t.Fatalf("unexpected message: %q", findings[0].Message)
	}
}

func TestOrphanMigrationTableRule_NoModels(t *testing.T) {
	t.Parallel()

	rule := &OrphanMigrationTableRule{}
	snap := schema.NewSnapshot()
	snap.ApplyCreateTable("users", []schema.ColumnDef{{Name: "id", Type: "integer"}}, "migrations/001.sql", 1)

	findings := rule.CheckSchema(snap, nil)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %+v", findings)
	}
}
