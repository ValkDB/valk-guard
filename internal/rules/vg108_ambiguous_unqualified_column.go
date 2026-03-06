// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// AmbiguousUnqualifiedColumnRule flags unqualified column usages that are
// present in multiple referenced tables.
type AmbiguousUnqualifiedColumnRule struct{}

// ID returns the unique rule identifier.
func (r *AmbiguousUnqualifiedColumnRule) ID() string { return "VG108" }

// Name returns the human-readable rule name.
func (r *AmbiguousUnqualifiedColumnRule) Name() string { return "ambiguous-unqualified-column" }

// Description explains what this rule checks.
func (r *AmbiguousUnqualifiedColumnRule) Description() string {
	return "Detects unqualified columns that are ambiguous across multiple referenced tables."
}

// DefaultSeverity returns the default severity for this rule.
func (r *AmbiguousUnqualifiedColumnRule) DefaultSeverity() Severity { return SeverityWarning }

// CheckQuerySchema validates unqualified column usage against resolved tables.
func (r *AmbiguousUnqualifiedColumnRule) CheckQuerySchema(
	_ context.Context,
	snap *schema.Snapshot,
	stmt *scanner.SQLStatement,
	parsed *postgresparser.ParsedQuery,
) []Finding {
	if parsed == nil || len(snap.Tables) == 0 {
		return nil
	}

	resolved := resolveQueryTables(snap, parsed)
	if len(resolved.unique) < 2 {
		return nil
	}

	seen := make(map[string]struct{})
	findings := make([]Finding, 0, len(parsed.ColumnUsage))

	for i := range parsed.ColumnUsage {
		usage := &parsed.ColumnUsage[i]
		contextLabel, ok := ambiguousUsageContext(usage.UsageType)
		if !ok {
			continue
		}
		if strings.TrimSpace(usage.TableAlias) != "" {
			continue
		}

		colName := normalizeUsageColumn(usage.Column)
		if colName == "" {
			continue
		}

		var owners []string
		for _, td := range resolved.unique {
			if _, exists := td.Columns[colName]; exists {
				owners = append(owners, td.Name)
			}
		}
		if len(owners) < 2 {
			continue
		}

		slices.Sort(owners)
		key := contextLabel + "|" + colName + "|" + strings.Join(owners, ",")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("%s column %q is ambiguous across tables %s; qualify it with a table alias", contextLabel, colName, strings.Join(owners, ", ")),
			stmt.File,
			stmt.Line,
			stmt.SQL,
		))
	}

	return findings
}

func ambiguousUsageContext(usageType postgresparser.ColumnUsageType) (string, bool) {
	switch usageType {
	case postgresparser.ColumnUsageTypeProjection:
		return "projection", true
	case postgresparser.ColumnUsageTypeFilter:
		return "filter", true
	case postgresparser.ColumnUsageTypeJoin:
		return "join", true
	case postgresparser.ColumnUsageTypeGroupBy:
		return "group-by", true
	case postgresparser.ColumnUsageTypeOrderBy:
		return "order-by", true
	default:
		return "", false
	}
}
