// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// UnknownProjectionColumnRule flags projected columns that are not present in
// the resolved schema table definition.
type UnknownProjectionColumnRule struct{}

// ID returns the unique rule identifier.
func (r *UnknownProjectionColumnRule) ID() string { return "VG105" }

// Name returns the human-readable rule name.
func (r *UnknownProjectionColumnRule) Name() string { return "unknown-projection-column" }

// Description explains what this rule checks.
func (r *UnknownProjectionColumnRule) Description() string {
	return "Detects projected columns that are not found in the migration schema."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnknownProjectionColumnRule) DefaultSeverity() Severity { return SeverityError }

// CheckQuerySchema validates SELECT projection column usages against the schema.
func (r *UnknownProjectionColumnRule) CheckQuerySchema(
	_ context.Context,
	snap *schema.Snapshot,
	stmt *scanner.SQLStatement,
	parsed *postgresparser.ParsedQuery,
) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect || len(snap.Tables) == 0 {
		return nil
	}

	resolved := resolveQueryTables(snap, parsed)
	if len(resolved.unique) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	findings := make([]Finding, 0, len(parsed.ColumnUsage))
	for i := range parsed.ColumnUsage {
		usage := &parsed.ColumnUsage[i]
		if usage.UsageType != postgresparser.ColumnUsageTypeProjection {
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

		key := td.Name + "|" + colName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("projection column %q not found in table %q schema; check SELECT list and schema/model mappings", colName, td.Name),
			stmt.File,
			stmt.Line,
			stmt.SQL,
		))
	}
	return findings
}
