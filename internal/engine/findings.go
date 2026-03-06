// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"cmp"
	"slices"
	"strings"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

// sortFindings sorts findings deterministically by file, line, column,
// rule ID, and message so that report output is stable across runs.
func sortFindings(findings []rules.Finding) {
	slices.SortFunc(findings, func(a, b rules.Finding) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		if c := cmp.Compare(a.EndLine, b.EndLine); c != 0 {
			return c
		}
		if c := cmp.Compare(a.EndColumn, b.EndColumn); c != 0 {
			return c
		}
		if c := strings.Compare(a.RuleID, b.RuleID); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
}

// applyStatementRange fills missing finding range fields from the originating
// statement so reporters can render multiline diagnostics consistently.
func applyStatementRange(findings []rules.Finding, stmt *scanner.SQLStatement) {
	startLine, startColumn, endLine, endColumn := normalizeStatementRange(stmt)
	for i, finding := range findings {
		if finding.Line < 1 {
			finding.Line = startLine
		}
		if finding.Column < 1 || (finding.Column == 1 && startColumn > 1 && finding.Line == startLine) {
			finding.Column = startColumn
		}
		if finding.EndLine < finding.Line {
			finding.EndLine = endLine
		}
		if finding.EndColumn < 1 {
			finding.EndColumn = endColumn
		}
		findings[i] = finding
	}
}

// normalizeStatementRange returns a valid 1-based range for a scanned
// statement, falling back to a minimal single-column span when needed.
func normalizeStatementRange(stmt *scanner.SQLStatement) (startL, startC, endL, endC int) {
	return rules.NormalizeRange(stmt.Line, stmt.Column, stmt.EndLine, stmt.EndColumn)
}

// queryFindingKeyValue is used for de-duplicating query-schema findings
// emitted from multiple schema snapshots.
type queryFindingKeyValue struct {
	RuleID    string
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Message   string
	SQL       string
}

// queryFindingKey builds a stable deduplication key for query-schema findings.
func queryFindingKey(f *rules.Finding) queryFindingKeyValue {
	return queryFindingKeyValue{
		RuleID:    f.RuleID,
		File:      f.File,
		Line:      f.Line,
		Column:    f.Column,
		EndLine:   f.EndLine,
		EndColumn: f.EndColumn,
		Message:   f.Message,
		SQL:       f.SQL,
	}
}
