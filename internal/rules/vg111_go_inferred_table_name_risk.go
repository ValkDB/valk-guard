// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/valkdb/valk-guard/internal/schema"
)

// GoInferredTableNameRiskRule flags Go models that rely on inferred mappings
// without an explicit table mapping.
type GoInferredTableNameRiskRule struct{}

// ID returns the unique rule identifier.
func (r *GoInferredTableNameRiskRule) ID() string { return "VG111" }

// Name returns the human-readable rule name.
func (r *GoInferredTableNameRiskRule) Name() string { return "go-inferred-table-name-risk" }

// Description explains what this rule checks.
func (r *GoInferredTableNameRiskRule) Description() string {
	return "Detects Go models that rely on inferred table/column mapping without explicit table declaration."
}

// DefaultSeverity returns the default severity for this rule.
func (r *GoInferredTableNameRiskRule) DefaultSeverity() Severity { return SeverityWarning }

// CheckSchema reports a warning when a Go model has inferred mappings and no
// explicit table mapping.
func (r *GoInferredTableNameRiskRule) CheckSchema(_ *schema.Snapshot, models []schema.ModelDef) []Finding {
	var findings []Finding

	for _, model := range models {
		if model.Source != schema.ModelSourceGo {
			continue
		}
		if model.TableExplicit {
			continue
		}

		hasInferredColumn := false
		for _, col := range model.Columns {
			if col.MappingKind == schema.MappingKindInferred {
				hasInferredColumn = true
				break
			}
		}
		if !hasInferredColumn {
			continue
		}

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf(
				"go model %q uses inferred table/column mapping; prefer explicit TableName()/column mapping",
				model.Table,
			),
			model.File,
			model.Line,
			"",
		))
	}

	return findings
}
