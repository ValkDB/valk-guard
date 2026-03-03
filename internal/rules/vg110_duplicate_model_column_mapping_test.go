// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func TestDuplicateModelColumnMappingRule(t *testing.T) {
	t.Parallel()

	rule := &DuplicateModelColumnMappingRule{}
	models := []schema.ModelDef{
		{
			Table:  "users",
			Source: schema.ModelSourceGo,
			File:   "go/models.go",
			Line:   3,
			Columns: []schema.ModelColumn{
				{Name: "id", Field: "ID", Line: 4},
				{Name: "email", Field: "Email", Line: 5},
				{Name: "id", Field: "LegacyID", Line: 6},
			},
		},
	}

	findings := rule.CheckSchema(nil, models)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].RuleID != "VG110" {
		t.Fatalf("RuleID = %q, want VG110", findings[0].RuleID)
	}
}

func TestDuplicateModelColumnMappingRule_NoDuplicates(t *testing.T) {
	t.Parallel()

	rule := &DuplicateModelColumnMappingRule{}
	models := []schema.ModelDef{
		{
			Table:  "users",
			Source: schema.ModelSourceGo,
			Columns: []schema.ModelColumn{
				{Name: "id", Field: "ID"},
				{Name: "email", Field: "Email"},
			},
		},
	}

	if findings := rule.CheckSchema(nil, models); len(findings) != 0 {
		t.Fatalf("expected no findings, got %+v", findings)
	}
}
