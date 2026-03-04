// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"

	"github.com/valkdb/postgresparser"
)

// UnboundedSelectRule detects SELECT statements without LIMIT/FETCH bounds.
type UnboundedSelectRule struct{}

// ID returns the unique rule identifier.
func (r *UnboundedSelectRule) ID() string { return "VG004" }

// Name returns the human-readable rule name.
func (r *UnboundedSelectRule) Name() string { return "unbounded-select" }

// Description explains what this rule checks.
func (r *UnboundedSelectRule) Description() string {
	return "Detects SELECT statements without LIMIT/FETCH bounds."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnboundedSelectRule) DefaultSeverity() Severity { return SeverityWarning }

// CommandTargets limits this rule to SELECT statements.
func (r *UnboundedSelectRule) CommandTargets() []postgresparser.QueryCommand {
	return []postgresparser.QueryCommand{postgresparser.QueryCommandSelect}
}

// Check reports a finding when a SELECT statement has no top-level LIMIT/FETCH.
func (r *UnboundedSelectRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect {
		return nil
	}
	if isSingleRowProjectionSelect(parsed) {
		return nil
	}
	if isSingleRowAggregateSelect(parsed) {
		return nil
	}
	if hasLimitClause(parsed) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"SELECT without LIMIT may return unbounded rows; add LIMIT or FETCH FIRST",
			file,
			line,
			rawSQL,
		),
	}
}

// isSingleRowProjectionSelect reports true for SELECT projections that do not
// read from any table source (for example: SELECT 1, SELECT now()).
func isSingleRowProjectionSelect(parsed *postgresparser.ParsedQuery) bool {
	return len(parsed.Tables) == 0
}

// isSingleRowAggregateSelect reports true when the query projection is entirely
// aggregate expressions without GROUP BY, which returns at most one row.
func isSingleRowAggregateSelect(parsed *postgresparser.ParsedQuery) bool {
	if len(parsed.Columns) == 0 || len(parsed.GroupBy) > 0 {
		return false
	}
	for _, col := range parsed.Columns {
		if !isAggregateProjection(col.Expression) {
			return false
		}
	}
	return true
}

func isAggregateProjection(expr string) bool {
	expr = strings.TrimSpace(strings.ToLower(expr))
	if expr == "" {
		return false
	}
	aggregatePrefixes := []string{
		"count(", "count (",
		"sum(", "sum (",
		"avg(", "avg (",
		"min(", "min (",
		"max(", "max (",
		"bool_and(", "bool_and (",
		"bool_or(", "bool_or (",
		"every(", "every (",
		"array_agg(", "array_agg (",
		"json_agg(", "json_agg (",
		"jsonb_agg(", "jsonb_agg (",
		"string_agg(", "string_agg (",
	}
	for _, prefix := range aggregatePrefixes {
		if strings.HasPrefix(expr, prefix) {
			return true
		}
	}
	return false
}
