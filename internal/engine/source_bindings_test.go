// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"slices"
	"testing"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// TestSourceConfigFiltersScanners verifies top-level source config controls
// which scanners participate in file discovery and execution.
func TestSourceConfigFiltersScanners(t *testing.T) {
	cfg := config.Default()
	cfg.Sources = map[string]bool{"csharp": false, "sqlalchemy": false}

	bindings := filterScannerBindings(cfg, defaultScannerBindings())
	for _, binding := range bindings {
		if binding.name == string(scanner.EngineCSharp) || binding.name == string(scanner.EngineSQLAlchemy) {
			t.Fatalf("expected %s scanner to be filtered", binding.name)
		}
	}
}

// TestSourceConfigFiltersModelExtractors verifies disabling all related source
// engines removes the matching model extractor from schema-aware analysis.
func TestSourceConfigFiltersModelExtractors(t *testing.T) {
	cfg := config.Default()
	cfg.Sources = map[string]bool{"sqlalchemy": false}

	bindings := filterModelBindings(cfg, defaultModelBindings(cfg))
	for _, binding := range bindings {
		for _, engine := range binding.queryEngines {
			if engine == scanner.EngineSQLAlchemy {
				t.Fatal("expected sqlalchemy model extractor to be filtered")
			}
		}
	}
}

func TestSourceBindingHelpers(t *testing.T) {
	cfg := config.Default()
	scanners := filterScannerBindings(cfg, defaultScannerBindings())
	models := filterModelBindings(cfg, defaultModelBindings(cfg))

	exts := requiredExtensions(scanners, models)
	for _, want := range []string{".cs", ".go", ".py", ".sql"} {
		if !slices.Contains(exts, want) {
			t.Fatalf("expected required extension %q in %v", want, exts)
		}
	}

	configEngines := sourceConfigEngines(models)
	if !slices.Contains(configEngines[schema.ModelSourceGo], scanner.EngineGo) {
		t.Fatalf("expected Go model source config engines to include go, got %+v", configEngines)
	}

	queryEngines := sourceQueryEngines(models)
	if !slices.Contains(queryEngines[scanner.EngineGoqu], schema.ModelSourceGo) {
		t.Fatalf("expected goqu query engine to use Go model source, got %+v", queryEngines)
	}

	inputs := newScannerInputs()
	seen := make(map[string]map[string]struct{})
	addInputFile(inputs, seen, ".go", "b.GO")
	addInputFile(inputs, seen, ".cs", "a.cs")
	addInputFile(inputs, seen, ".txt", "ignored.txt")
	files := inputs.filesForExtensions([]string{"go", ".cs"})
	if want := []string{"a.cs", "b.GO"}; !slices.Equal(files, want) {
		t.Fatalf("filesForExtensions() = %+v, want %+v", files, want)
	}

	if got := normalizeExt("CS"); got != ".cs" {
		t.Fatalf("normalizeExt() = %q, want .cs", got)
	}
}

func TestSourceBindingAllDisabledKeepsRawSQLDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Sources = map[string]bool{
		"sql":        false,
		"go":         false,
		"goqu":       false,
		"sqlalchemy": false,
		"csharp":     false,
	}

	if scanners := filterScannerBindings(cfg, defaultScannerBindings()); len(scanners) != 0 {
		t.Fatalf("expected all scanners disabled, got %+v", scanners)
	}
	if models := filterModelBindings(cfg, defaultModelBindings(cfg)); len(models) != 0 {
		t.Fatalf("expected all model extractors disabled, got %+v", models)
	}
}
