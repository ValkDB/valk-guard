// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
)

func TestReviewdogSeverity(t *testing.T) {
	tests := []struct {
		severity rules.Severity
		want     string
	}{
		{severity: rules.SeverityError, want: "ERROR"},
		{severity: rules.SeverityWarning, want: "WARNING"},
		{severity: rules.SeverityInfo, want: "INFO"},
	}

	for _, tt := range tests {
		if got := reviewdogSeverity(tt.severity); got != tt.want {
			t.Fatalf("reviewdogSeverity(%q) = %q, want %q", tt.severity, got, tt.want)
		}
	}
}

func TestReviewdogMessage(t *testing.T) {
	t.Run("synthetic SQL strips prefix and adds origin", func(t *testing.T) {
		msg := reviewdogMessage(rules.Finding{
			RuleID:  "VG004",
			Message: "SELECT without LIMIT may return unbounded rows; add LIMIT or FETCH FIRST",
			SQL:     `/* valk-guard:synthetic sqlalchemy-ast */ SELECT "User"."id", "User"."email" FROM "User" WHERE "User"."active" = TRUE`,
		})

		if want := "Origin: SQLAlchemy query builder"; !bytes.Contains([]byte(msg), []byte(want)) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
		if bytes.Contains([]byte(msg), []byte("valk-guard:synthetic")) {
			t.Fatalf("expected synthetic prefix to be stripped, got %q", msg)
		}
	})

	t.Run("raw SQL keeps compact preview only", func(t *testing.T) {
		msg := reviewdogMessage(rules.Finding{
			RuleID:  "VG004",
			Message: "SELECT without LIMIT may return unbounded rows; add LIMIT or FETCH FIRST",
			SQL:     "WITH active_users AS (\n  SELECT id FROM users\n)\nSELECT id FROM active_users",
		})

		if want := "Query: `WITH active_users AS (`"; !bytes.Contains([]byte(msg), []byte(want)) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
		if bytes.Contains([]byte(msg), []byte("Origin:")) {
			t.Fatalf("expected no origin hint for raw SQL, got %q", msg)
		}
	})
}

func TestBuildRDJSONLDiagnostic(t *testing.T) {
	t.Run("single-line fallback range", func(t *testing.T) {
		diag := buildRDJSONLDiagnostic(rules.Finding{
			RuleID:   "VG001",
			Severity: rules.SeverityWarning,
			Message:  "avoid SELECT *",
			File:     "query.sql",
			Line:     12,
			Column:   3,
			SQL:      "SELECT * FROM users",
		})

		if diag.Source.Name != "valk-guard" {
			t.Fatalf("unexpected source name: %q", diag.Source.Name)
		}
		if diag.Code.Value != "VG001" {
			t.Fatalf("unexpected code value: %q", diag.Code.Value)
		}
		if diag.Location.Path != "query.sql" {
			t.Fatalf("unexpected path: %q", diag.Location.Path)
		}
		if diag.Location.Range.Start.Line != 12 || diag.Location.Range.Start.Column != 3 {
			t.Fatalf("unexpected start position: %+v", diag.Location.Range.Start)
		}
		if diag.Location.Range.End.Line != 12 || diag.Location.Range.End.Column != 4 {
			t.Fatalf("unexpected end position: %+v", diag.Location.Range.End)
		}
	})

	t.Run("multiline finding range is preserved", func(t *testing.T) {
		diag := buildRDJSONLDiagnostic(rules.Finding{
			RuleID:    "VG106",
			Severity:  rules.SeverityError,
			Message:   "unknown filter column",
			File:      "complex_queries.sql",
			Line:      21,
			Column:    1,
			EndLine:   26,
			EndColumn: 9,
			SQL:       "SELECT users.id\nFROM users\nWHERE orders.ghost_status = 'pending'\nLIMIT 10",
		})

		if diag.Location.Range.Start.Line != 21 || diag.Location.Range.Start.Column != 1 {
			t.Fatalf("unexpected start position: %+v", diag.Location.Range.Start)
		}
		if diag.Location.Range.End.Line != 26 || diag.Location.Range.End.Column != 9 {
			t.Fatalf("unexpected end position: %+v", diag.Location.Range.End)
		}
	})
}

func TestRDJSONLReporterReport(t *testing.T) {
	reporter := &RDJSONLReporter{}
	var buf bytes.Buffer

	err := reporter.Report(context.Background(), &buf, []rules.Finding{
		{
			RuleID:    "VG004",
			Severity:  rules.SeverityWarning,
			Message:   "SELECT without LIMIT may return unbounded rows; add LIMIT or FETCH FIRST",
			File:      "queries.py",
			Line:      23,
			Column:    1,
			EndLine:   26,
			EndColumn: 2,
			SQL:       `/* valk-guard:synthetic sqlalchemy-ast */ SELECT "User"."id" FROM "User"`,
		},
	})
	if err != nil {
		t.Fatalf("unexpected report error: %v", err)
	}

	var diag rdjsonlDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatalf("invalid rdjsonl output: %v", err)
	}
	if diag.Severity != "WARNING" {
		t.Fatalf("unexpected severity: %q", diag.Severity)
	}
	if diag.Location.Path != "queries.py" {
		t.Fatalf("unexpected path: %q", diag.Location.Path)
	}
	if diag.Location.Range.End.Line != 26 {
		t.Fatalf("expected multiline range to be preserved, got %+v", diag.Location.Range)
	}
	if want := "Origin: SQLAlchemy query builder"; !bytes.Contains([]byte(diag.Message), []byte(want)) {
		t.Fatalf("expected message to contain %q, got %q", want, diag.Message)
	}
}
