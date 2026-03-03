package rules

import (
	"fmt"

	"github.com/valkdb/valk-guard/internal/schema"
)

// TableNotFoundRule flags models that map to a table with no CREATE TABLE in
// the migration schema.
type TableNotFoundRule struct{}

// ID returns the unique rule identifier.
func (r *TableNotFoundRule) ID() string { return "VG104" }

// Name returns the human-readable rule name.
func (r *TableNotFoundRule) Name() string { return "table-not-found" }

// Description explains what this rule checks.
func (r *TableNotFoundRule) Description() string {
	return "Detects models that map to a table not found in migration DDL."
}

// DefaultSeverity returns the default severity for this rule.
func (r *TableNotFoundRule) DefaultSeverity() Severity { return SeverityError }

// CheckSchema checks that every model maps to a known table.
func (r *TableNotFoundRule) CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding {
	if len(snap.Tables) == 0 {
		return nil
	}
	var findings []Finding
	for _, model := range models {
		if !model.TableExplicit {
			continue
		}
		if matchTable(snap, model.Table) == nil {
			findings = append(findings, newFinding(
				r.ID(),
				r.DefaultSeverity(),
				fmt.Sprintf("model %q maps to table %q which has no CREATE TABLE in migrations", model.Table, model.Table),
				model.File,
				model.Line,
				"",
			))
		}
	}
	return findings
}
