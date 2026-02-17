package rules

import (
	"fmt"

	"github.com/valkdb/postgresparser"
)

// NonConcurrentIndexRule detects CREATE INDEX statements without CONCURRENTLY.
type NonConcurrentIndexRule struct{}

// ID returns the unique rule identifier.
func (r *NonConcurrentIndexRule) ID() string { return "VG008" }

// Name returns the human-readable rule name.
func (r *NonConcurrentIndexRule) Name() string { return "non-concurrent-index" }

// Description explains what this rule checks.
func (r *NonConcurrentIndexRule) Description() string {
	return "Detects CREATE INDEX statements that omit CONCURRENTLY."
}

// DefaultSeverity returns the default severity for this rule.
func (r *NonConcurrentIndexRule) DefaultSeverity() Severity { return SeverityWarning }

// Check reports findings for CREATE INDEX operations missing CONCURRENTLY.
func (r *NonConcurrentIndexRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandDDL {
		return nil
	}

	var findings []Finding
	for _, action := range parsed.DDLActions {
		if action.Type != postgresparser.DDLCreateIndex || hasFlag(action.Flags, "CONCURRENTLY") {
			continue
		}

		message := "CREATE INDEX without CONCURRENTLY may block writes"
		if action.ObjectName != "" {
			message = fmt.Sprintf("CREATE INDEX %s without CONCURRENTLY may block writes", action.ObjectName)
		}
		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			message,
			file,
			line,
			rawSQL,
		))
	}

	return findings
}
