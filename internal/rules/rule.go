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
	RuleID   string   `json:"rule_id"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	SQL      string   `json:"sql,omitempty"`
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
