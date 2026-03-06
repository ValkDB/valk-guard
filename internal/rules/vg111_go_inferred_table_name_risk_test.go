// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestGoInferredTableNameRiskRule(t *testing.T) {
	t.Parallel()

	rule := &GoInferredTableNameRiskRule{}
	models := []schema.ModelDef{
		{
			Table:         "users",
			TableExplicit: false,
			Source:        schema.ModelSourceGo,
			File:          "go/models.go",
			Line:          10,
			Columns: []schema.ModelColumn{
				{Name: "id", Field: "ID", MappingKind: schema.MappingKindInferred},
			},
		},
	}

	findings := rule.CheckSchema(context.Background(), nil, models)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].RuleID != "VG111" {
		t.Fatalf("RuleID = %q, want VG111", findings[0].RuleID)
	}
}

func TestGoInferredTableNameRiskRule_SkipsExplicitOrNonGo(t *testing.T) {
	t.Parallel()

	rule := &GoInferredTableNameRiskRule{}
	models := []schema.ModelDef{
		{
			Table:         "users",
			TableExplicit: true,
			Source:        schema.ModelSourceGo,
			Columns: []schema.ModelColumn{
				{Name: "id", MappingKind: schema.MappingKindInferred},
			},
		},
		{
			Table:         "users",
			TableExplicit: false,
			Source:        schema.ModelSourceGo,
			Columns: []schema.ModelColumn{
				{Name: "id", MappingKind: schema.MappingKindExplicit},
			},
		},
		{
			Table:         "users",
			TableExplicit: false,
			Source:        schema.ModelSourceSQLAlchemy,
			Columns: []schema.ModelColumn{
				{Name: "id", MappingKind: schema.MappingKindInferred},
			},
		},
	}

	if findings := rule.CheckSchema(context.Background(), nil, models); len(findings) != 0 {
		t.Fatalf("expected no findings, got %+v", findings)
	}
}
