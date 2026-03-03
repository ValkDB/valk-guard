package rules

import (
	"fmt"
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
		modelCols := make(map[string]bool, len(model.Columns))
		for _, mc := range model.Columns {
			modelCols[strings.ToLower(mc.Name)] = true
		}
		for colName, col := range td.Columns {
			if !col.Nullable && !col.HasDefault {
				if !modelCols[colName] {
					findings = append(findings, newFinding(
						r.ID(),
						r.DefaultSeverity(),
						fmt.Sprintf("table %q has NOT NULL column %q but model %q does not define it", td.Name, col.Name, model.Table),
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
