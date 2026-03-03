package rules

import (
	"fmt"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// UnknownFilterColumnRule flags WHERE/HAVING/JOIN predicate columns that are
// not present in the resolved schema table definition.
type UnknownFilterColumnRule struct{}

// ID returns the unique rule identifier.
func (r *UnknownFilterColumnRule) ID() string { return "VG106" }

// Name returns the human-readable rule name.
func (r *UnknownFilterColumnRule) Name() string { return "unknown-filter-column" }

// Description explains what this rule checks.
func (r *UnknownFilterColumnRule) Description() string {
	return "Detects predicate columns in WHERE/HAVING/JOIN clauses that are not found in the migration schema."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnknownFilterColumnRule) DefaultSeverity() Severity { return SeverityError }

// CheckQuerySchema validates filter and join column usages against the schema.
func (r *UnknownFilterColumnRule) CheckQuerySchema(
	snap *schema.Snapshot,
	stmt scanner.SQLStatement,
	parsed *postgresparser.ParsedQuery,
) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect || len(snap.Tables) == 0 {
		return nil
	}

	resolved := resolveQueryTables(snap, parsed)
	if len(resolved.unique) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var findings []Finding
	for _, usage := range parsed.ColumnUsage {
		if usage.UsageType != postgresparser.ColumnUsageTypeFilter &&
			usage.UsageType != postgresparser.ColumnUsageTypeJoin {
			continue
		}
		colName := normalizeUsageColumn(usage.Column)
		if colName == "" {
			continue
		}

		td, ok := resolveUsageTable(usage, resolved)
		if !ok {
			continue
		}
		if _, exists := td.Columns[colName]; exists {
			continue
		}

		contextLabel := "filter"
		if usage.UsageType == postgresparser.ColumnUsageTypeJoin {
			contextLabel = "join"
		}
		key := contextLabel + "|" + td.Name + "|" + colName
		if seen[key] {
			continue
		}
		seen[key] = true

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("%s predicate column %q not found in table %q schema", contextLabel, colName, td.Name),
			stmt.File,
			stmt.Line,
			stmt.SQL,
		))
	}
	return findings
}
