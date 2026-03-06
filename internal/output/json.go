// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"context"
	"encoding/json"
	"io"

	"github.com/valkdb/valk-guard/internal/rules"
)

// jsonReport is the top-level envelope for JSON output.
type jsonReport struct {
	Version  string      `json:"version"`
	Findings []jsonEntry `json:"findings"`
	Summary  jsonSummary `json:"summary"`
}

// jsonEntry mirrors rules.Finding with stable JSON field names.
type jsonEntry struct {
	RuleID    string         `json:"rule_id"`
	Severity  rules.Severity `json:"severity"`
	Message   string         `json:"message"`
	File      string         `json:"file"`
	Line      int            `json:"line"`
	Column    int            `json:"column"`
	EndLine   int            `json:"end_line,omitempty"`
	EndColumn int            `json:"end_column,omitempty"`
	SQL       string         `json:"sql,omitempty"`
}

// jsonSummary holds aggregate counts for the report.
type jsonSummary struct {
	Total int `json:"total"`
}

// JSONReporter outputs findings wrapped in a versioned JSON envelope.
// Version is the schema version of the JSON output format.
type JSONReporter struct {
	Version string
}

// Report writes findings as JSON with a metadata envelope.
func (r *JSONReporter) Report(ctx context.Context, w io.Writer, findings []rules.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entries := make([]jsonEntry, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, jsonEntry{
			RuleID:    f.RuleID,
			Severity:  f.Severity,
			Message:   f.Message,
			File:      f.File,
			Line:      f.Line,
			Column:    f.Column,
			EndLine:   f.EndLine,
			EndColumn: f.EndColumn,
			SQL:       f.SQL,
		})
	}

	ver := r.Version
	if ver == "" {
		ver = "1.0.0"
	}

	report := jsonReport{
		Version:  ver,
		Findings: entries,
		Summary:  jsonSummary{Total: len(entries)},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
