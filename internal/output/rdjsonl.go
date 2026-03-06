// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/valkdb/valk-guard/internal/rules"
)

const rulesHelpURL = "https://github.com/ValkDB/valk-guard"

type rdjsonlSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type rdjsonlCode struct {
	Value string `json:"value"`
}

type rdjsonlPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type rdjsonlRange struct {
	Start rdjsonlPosition `json:"start"`
	End   rdjsonlPosition `json:"end"`
}

type rdjsonlLocation struct {
	Path  string       `json:"path"`
	Range rdjsonlRange `json:"range"`
}

type rdjsonlDiagnostic struct {
	Source   rdjsonlSource   `json:"source"`
	Severity string          `json:"severity"`
	Code     rdjsonlCode     `json:"code"`
	Message  string          `json:"message"`
	Location rdjsonlLocation `json:"location"`
}

// RDJSONLReporter writes reviewdog-compatible diagnostics as newline-delimited
// JSON objects.
type RDJSONLReporter struct{}

// Report writes findings in reviewdog rdjsonl format.
func (r *RDJSONLReporter) Report(ctx context.Context, w io.Writer, findings []rules.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	for _, f := range findings {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := enc.Encode(buildRDJSONLDiagnostic(f)); err != nil {
			return err
		}
	}
	return nil
}

// buildRDJSONLDiagnostic converts a single finding into the reviewdog rdjsonl
// diagnostic shape, preserving multiline ranges when available.
func buildRDJSONLDiagnostic(f rules.Finding) rdjsonlDiagnostic {
	start, end := reviewdogRange(f)

	return rdjsonlDiagnostic{
		Source: rdjsonlSource{
			Name: "valk-guard",
			URL:  rulesHelpURL,
		},
		Severity: reviewdogSeverity(f.Severity),
		Code:     rdjsonlCode{Value: f.RuleID},
		Message:  reviewdogMessage(f),
		Location: rdjsonlLocation{
			Path: f.File,
			Range: rdjsonlRange{
				Start: start,
				End:   end,
			},
		},
	}
}

// reviewdogRange returns a valid 1-based range for reviewdog, falling back to
// a minimal single-column span when the finding does not provide an end range.
func reviewdogRange(f rules.Finding) (rdjsonlPosition, rdjsonlPosition) {
	line := f.Line
	if line < 1 {
		line = 1
	}

	column := f.Column
	if column < 1 {
		column = 1
	}

	endLine := f.EndLine
	if endLine < line {
		endLine = line
	}

	endColumn := f.EndColumn
	if endColumn < 1 {
		if endLine > line {
			endColumn = 1
		} else {
			endColumn = column + 1
		}
	}

	return rdjsonlPosition{Line: line, Column: column}, rdjsonlPosition{Line: endLine, Column: endColumn}
}

// reviewdogSeverity maps valk-guard severities to reviewdog severity levels.
func reviewdogSeverity(sev rules.Severity) string {
	switch sev {
	case rules.SeverityError:
		return "ERROR"
	case rules.SeverityWarning:
		return "WARNING"
	default:
		return "INFO"
	}
}

// reviewdogMessage builds a compact review comment message with a cleaned query
// preview and an origin hint for synthetic builder-derived SQL.
func reviewdogMessage(f rules.Finding) string {
	rendered := renderSQL(f.SQL)
	parts := []string{f.RuleID + ": " + f.Message}

	if origin := syntheticOriginLabel(rendered.SyntheticSource); origin != "" {
		parts = append(parts, "Origin: "+origin)
	}
	if query := previewLine(rendered.Cleaned, reviewdogPreviewMaxLen); query != "" {
		parts = append(parts, "Query: `"+query+"`")
	}

	return strings.Join(parts, " | ")
}
