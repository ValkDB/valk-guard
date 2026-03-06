// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

func TestModelsForRuleExcludesUnmappedSources(t *testing.T) {
	cfg := config.Default()

	// Only ModelSourceGo has a binding; ModelSourceSQLAlchemy is unmapped.
	bindings := []modelBinding{
		{
			source:        schema.ModelSourceGo,
			configEngines: []scanner.Engine{scanner.EngineGo},
		},
	}

	models := []schema.ModelDef{
		{Source: schema.ModelSourceGo, Table: "users"},
		{Source: schema.ModelSourceSQLAlchemy, Table: "orders"},
	}

	result := modelsForRule(models, cfg, "VG007", bindings)

	if len(result) != 1 {
		t.Fatalf("expected 1 model, got %d", len(result))
	}
	if result[0].Table != "users" {
		t.Fatalf("expected model table 'users', got %q", result[0].Table)
	}
}

func TestModelsForRuleIncludesMappedSource(t *testing.T) {
	cfg := config.Default()

	bindings := []modelBinding{
		{
			source:        schema.ModelSourceGo,
			configEngines: []scanner.Engine{scanner.EngineGo},
		},
		{
			source:        schema.ModelSourceSQLAlchemy,
			configEngines: []scanner.Engine{scanner.EngineSQLAlchemy},
		},
	}

	models := []schema.ModelDef{
		{Source: schema.ModelSourceGo, Table: "users"},
		{Source: schema.ModelSourceSQLAlchemy, Table: "orders"},
	}

	result := modelsForRule(models, cfg, "VG007", bindings)

	if len(result) != 2 {
		t.Fatalf("expected 2 models, got %d", len(result))
	}

	tables := map[string]bool{}
	for _, m := range result {
		tables[m.Table] = true
	}
	if !tables["users"] {
		t.Fatal("expected 'users' model to be included")
	}
	if !tables["orders"] {
		t.Fatal("expected 'orders' model to be included")
	}
}
