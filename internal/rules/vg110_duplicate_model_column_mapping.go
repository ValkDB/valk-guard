// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// DuplicateModelColumnMappingRule flags duplicate column mappings within a
// single model definition.
type DuplicateModelColumnMappingRule struct{}

// ID returns the unique rule identifier.
func (r *DuplicateModelColumnMappingRule) ID() string { return "VG110" }

// Name returns the human-readable rule name.
func (r *DuplicateModelColumnMappingRule) Name() string { return "duplicate-model-column-mapping" }

// Description explains what this rule checks.
func (r *DuplicateModelColumnMappingRule) Description() string {
	return "Detects models that map the same database column more than once."
}

// DefaultSeverity returns the default severity for this rule.
func (r *DuplicateModelColumnMappingRule) DefaultSeverity() Severity { return SeverityWarning }

// CheckSchema validates each model for duplicate column mappings.
func (r *DuplicateModelColumnMappingRule) CheckSchema(_ *schema.Snapshot, models []schema.ModelDef) []Finding {
	var findings []Finding

	for _, model := range models {
		seen := make(map[string]schema.ModelColumn)
		for _, col := range model.Columns {
			key := strings.ToLower(strings.TrimSpace(col.Name))
			if key == "" {
				continue
			}

			first, exists := seen[key]
			if !exists {
				seen[key] = col
				continue
			}

			findings = append(findings, newFinding(
				r.ID(),
				r.DefaultSeverity(),
				fmt.Sprintf(
					"model %q maps column %q multiple times (fields %q and %q)",
					model.Table,
					col.Name,
					first.Field,
					col.Field,
				),
				model.File,
				col.Line,
				"",
			))
		}
	}

	return findings
}
