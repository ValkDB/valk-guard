package scanner

import "strings"

// DisableAll is a sentinel value indicating all rules are disabled.
const DisableAll = "*"

// Directive represents a parsed inline ignore directive.
type Directive struct {
	Line    int      // 1-based line number where the directive appears.
	RuleIDs []string // Rule IDs to disable (e.g., ["VG001", "VG002"]).
}

// ParseDirectives scans source lines for valk-guard disable directives.
// Directives must appear at the start of a comment (after trimming whitespace),
// so they are not matched inside string literals or mid-line content.
// Supported formats:
//   - SQL:    -- valk-guard:disable VG001,VG002
//   - Go:     // valk-guard:disable VG001,VG002
//   - Python: # valk-guard:disable VG001,VG002
//
// A bare directive without rule IDs disables all rules for the next statement.
func ParseDirectives(lines []string) []Directive {
	var directives []Directive

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		var after string

		switch {
		case strings.HasPrefix(trimmed, "-- valk-guard:disable"):
			after = strings.TrimSpace(trimmed[len("-- valk-guard:disable"):])
		case strings.HasPrefix(trimmed, "// valk-guard:disable"):
			after = strings.TrimSpace(trimmed[len("// valk-guard:disable"):])
		case strings.HasPrefix(trimmed, "# valk-guard:disable"):
			after = strings.TrimSpace(trimmed[len("# valk-guard:disable"):])
		default:
			continue
		}

		var ruleIDs []string
		if after == "" {
			ruleIDs = []string{DisableAll}
		} else {
			for _, part := range strings.Split(after, ",") {
				id := strings.TrimSpace(part)
				if id != "" {
					ruleIDs = append(ruleIDs, id)
				}
			}
		}

		if len(ruleIDs) > 0 {
			directives = append(directives, Directive{
				Line:    i + 1, // convert 0-based slice index to 1-based line number
				RuleIDs: ruleIDs,
			})
		}
	}

	return directives
}

// DisabledRulesForLine returns the rule IDs disabled by directives on
// the given line or the immediately preceding line.
func DisabledRulesForLine(directives []Directive, line int) []string {
	var disabled []string
	for _, d := range directives {
		if d.Line == line || d.Line == line-1 {
			disabled = append(disabled, d.RuleIDs...)
		}
	}
	return disabled
}

// IsDisabled returns true if the given rule ID is in the disabled list,
// or if the list contains the DisableAll sentinel ("*").
func IsDisabled(ruleID string, disabled []string) bool {
	for _, id := range disabled {
		if id == DisableAll || id == ruleID {
			return true
		}
	}
	return false
}
