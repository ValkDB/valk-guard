package output

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/valkdb/valk-guard/internal/rules"
)

// ANSI escape codes used for colorized terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

// TerminalReporter prints findings in a human-readable format.
type TerminalReporter struct {
	NoColor bool
}

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

		sev := r.colorize(string(f.Severity), f.Severity)
		_, err := fmt.Fprintf(w, "%s:%d: %s [%s] %s\n", f.File, f.Line, sev, f.RuleID, f.Message)
		if err != nil {
			return err
		}
	}

	noun := "findings"
	if len(findings) == 1 {
		noun = "finding"
	}
	_, err := fmt.Fprintf(w, "\n%d %s\n", len(findings), noun)
	return err
}

// colorize wraps text with the appropriate ANSI color code for the given
// severity. It returns plain text when color is disabled via NoColor or
// the NO_COLOR environment variable.
func (r *TerminalReporter) colorize(text string, sev rules.Severity) string {
	if r.NoColor || os.Getenv("NO_COLOR") != "" {
		return text
	}

	var color string
	switch sev {
	case rules.SeverityError:
		color = colorRed
	case rules.SeverityWarning:
		color = colorYellow
	case rules.SeverityInfo:
		color = colorCyan
	default:
		return text
	}

	return color + text + colorReset
}
