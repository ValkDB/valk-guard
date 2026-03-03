// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// UnknownFilterColumnRule flags WHERE/HAVING/JOIN/GROUP BY/ORDER BY columns
// that are not present in the resolved schema table definition.
type UnknownFilterColumnRule struct{}

// ID returns the unique rule identifier.
func (r *UnknownFilterColumnRule) ID() string { return "VG106" }

// Name returns the human-readable rule name.
func (r *UnknownFilterColumnRule) Name() string { return "unknown-filter-column" }

// Description explains what this rule checks.
func (r *UnknownFilterColumnRule) Description() string {
	return "Detects columns in WHERE/HAVING/JOIN/GROUP BY/ORDER BY clauses that are not found in the migration schema."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnknownFilterColumnRule) DefaultSeverity() Severity { return SeverityError }

// CheckQuerySchema validates filter and join column usages against the schema.
func (r *UnknownFilterColumnRule) CheckQuerySchema(
	snap *schema.Snapshot,
	stmt scanner.SQLStatement,
	parsed *postgresparser.ParsedQuery,
) []Finding {
	if parsed == nil || len(snap.Tables) == 0 {
		return nil
	}
	// Filter/join column validation applies to SELECT, UPDATE, and DELETE commands.
	switch parsed.Command {
	case postgresparser.QueryCommandSelect, postgresparser.QueryCommandUpdate, postgresparser.QueryCommandDelete:
		// proceed
	default:
		return nil
	}

	resolved := resolveQueryTables(snap, parsed)
	if len(resolved.unique) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var findings []Finding
	for _, usage := range parsed.ColumnUsage {
		var contextLabel string
		switch usage.UsageType {
		case postgresparser.ColumnUsageTypeFilter:
			contextLabel = "filter"
		case postgresparser.ColumnUsageTypeJoin:
			contextLabel = "join"
		case postgresparser.ColumnUsageTypeGroupBy:
			contextLabel = "group-by"
		case postgresparser.ColumnUsageTypeOrderBy:
			contextLabel = "order-by"
		default:
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

		key := contextLabel + "|" + td.Name + "|" + colName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("%s predicate column %q not found in table %q schema; check predicate/group/order columns in schema/model mappings", contextLabel, colName, td.Name),
			stmt.File,
			stmt.Line,
			stmt.SQL,
		))
	}
	return findings
}
