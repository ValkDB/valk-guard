// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/scanner"
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
