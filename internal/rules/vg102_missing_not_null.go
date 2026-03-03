// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// MissingNotNullRule flags NOT NULL columns (without defaults) that are missing
// from the ORM model definition.
type MissingNotNullRule struct{}

// ID returns the unique rule identifier.
func (r *MissingNotNullRule) ID() string { return "VG102" }

// Name returns the human-readable rule name.
func (r *MissingNotNullRule) Name() string { return "missing-not-null" }

// Description explains what this rule checks.
func (r *MissingNotNullRule) Description() string {
	return "Detects NOT NULL columns without defaults that the model does not define."
}

// DefaultSeverity returns the default severity for this rule.
func (r *MissingNotNullRule) DefaultSeverity() Severity { return SeverityWarning }

// CheckSchema compares schema columns against model definitions.
func (r *MissingNotNullRule) CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding {
	var findings []Finding
	for _, model := range models {
		td := matchTable(snap, model.Table)
		if td == nil {
			continue
		}
		// Build set of model column names (lowercase).
		modelCols := make(map[string]struct{}, len(model.Columns))
		for _, mc := range model.Columns {
			modelCols[strings.ToLower(mc.Name)] = struct{}{}
		}
		// Collect and sort column names for deterministic output.
		colNames := make([]string, 0, len(td.Columns))
		for colName := range td.Columns {
			colNames = append(colNames, colName)
		}
		slices.Sort(colNames)
		for _, colName := range colNames {
			col := td.Columns[colName]
			if !col.Nullable && !col.HasDefault {
				if _, ok := modelCols[colName]; !ok {
					findings = append(findings, newFinding(
						r.ID(),
						r.DefaultSeverity(),
						fmt.Sprintf("table %q has NOT NULL column %q but model %q does not define it; add the field or make the migration column nullable/defaulted", td.Name, col.Name, model.Table),
						model.File,
						model.Line,
						"",
					))
				}
			}
		}
	}
	return findings
}
