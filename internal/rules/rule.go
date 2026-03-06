// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "github.com/valkdb/postgresparser"

// Severity represents the severity level of a finding.
type Severity string

// Severity constants for rule findings.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding represents a single lint finding produced by a rule.
type Finding struct {
	RuleID    string   `json:"rule_id"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Column    int      `json:"column"`
	EndLine   int      `json:"end_line,omitempty"`
	EndColumn int      `json:"end_column,omitempty"`
	SQL       string   `json:"sql,omitempty"`
}

// Rule is the interface that all valk-guard lint rules must implement.
type Rule interface {
	// ID returns the unique identifier for this rule (e.g., "VG001").
	ID() string

	// Name returns a human-readable name for this rule.
	Name() string

	// Description returns a longer explanation of what this rule checks.
	Description() string

	// DefaultSeverity returns the default severity if not overridden by config.
	DefaultSeverity() Severity

	// Check examines a parsed query and returns any findings.
	Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding
}

// NormalizeRange returns a valid 1-based range, falling back to minimal
// single-column spans when needed.
func NormalizeRange(line, column, endLine, endColumn int) (startL, startC, endL, endC int) {
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	if endLine < line {
		endLine = line
	}
	if endColumn < 1 {
		if endLine > line {
			endColumn = 1
		} else {
			endColumn = column + 1
		}
	}
	return line, column, endLine, endColumn
}

// CommandTargetedRule is an optional interface for rules that only apply to
// specific SQL command types. Rules that do not implement this interface are
// treated as cross-cutting and run for all command types.
type CommandTargetedRule interface {
	CommandTargets() []postgresparser.QueryCommand
}
