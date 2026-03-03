// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"context"
	"fmt"
	"io"

	"github.com/valkdb/valk-guard/internal/rules"
)

// TerminalReporter prints findings in a human-readable format.
type TerminalReporter struct{}

// Report writes findings to the terminal.
func (r *TerminalReporter) Report(ctx context.Context, w io.Writer, findings []rules.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "0 findings")
		return err
	}

	for _, f := range findings {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, err := fmt.Fprintf(w, "%s:%d: %s [%s] %s\n", f.File, f.Line, string(f.Severity), f.RuleID, f.Message)
		if err != nil {
			return err
		}

		if f.SQL != "" {
			snippet := f.SQL
			if len(snippet) > 120 {
				snippet = snippet[:120] + "..."
			}
			if _, err := fmt.Fprintf(w, "    %s\n", snippet); err != nil {
				return err
			}
		}
	}

	noun := "findings"
	if len(findings) == 1 {
		noun = "finding"
	}
	_, err := fmt.Fprintf(w, "\n%d %s\n", len(findings), noun)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, "Suppress findings with:\n"+
		"  SQL: -- valk-guard:disable <RULE_ID>\n"+
		"  Go:  // valk-guard:disable <RULE_ID>\n"+
		"  Py:  # valk-guard:disable <RULE_ID>\n")
	return err
}
