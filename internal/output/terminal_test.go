// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestTerminalReporter(t *testing.T) {
	errorFinding := rules.Finding{
		RuleID:   "VG001",
		Severity: rules.SeverityError,
		Message:  "avoid SELECT *",
		File:     "test.sql",
		Line:     10,
		SQL:      "SELECT * FROM users",
	}

	tests := []struct {
		name         string
		findings     []rules.Finding
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "no findings",
			findings:     nil,
			wantContains: []string{"0 findings"},
		},
		{
			name:     "single finding with correct format",
			findings: []rules.Finding{errorFinding},
			wantContains: []string{
				"test.sql:10",
				"[VG001]",
				"1 finding",
				"Suppress findings with:",
				"  SQL: -- valk-guard:disable <RULE_ID>",
				"  Go:  // valk-guard:disable <RULE_ID>",
				"  Py:  # valk-guard:disable <RULE_ID>",
				"    SELECT * FROM users",
			},
			wantAbsent: []string{"1 findings"},
		},
		{
			name: "finding without SQL omits snippet line",
			findings: []rules.Finding{
				{RuleID: "VG002", Severity: rules.SeverityError, Message: "missing WHERE", File: "x.sql", Line: 1},
			},
			wantContains: []string{"x.sql:1", "[VG002]"},
			wantAbsent:   []string{"    "},
		},
		{
			name: "long SQL is truncated",
			findings: []rules.Finding{
				{RuleID: "VG001", Severity: rules.SeverityWarning, Message: "avoid SELECT *", File: "big.sql", Line: 1, SQL: "SELECT " + strings.Repeat("a", 200) + " FROM t"},
			},
			wantContains: []string{"..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &TerminalReporter{}
			var buf bytes.Buffer

			if err := r.Report(context.Background(), &buf, tt.findings); err != nil {
				t.Fatalf("report error: %v", err)
			}

			out := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("expected output to contain %q, got %q", want, out)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("expected output to NOT contain %q, got %q", absent, out)
				}
			}
		})
	}
}

func TestTerminalReportGolden(t *testing.T) {
	findings := []rules.Finding{
		{RuleID: "VG001", Severity: rules.SeverityWarning, Message: "avoid SELECT *", File: "query.sql", Line: 10, SQL: "SELECT * FROM users WHERE active = true"},
		{RuleID: "VG002", Severity: rules.SeverityError, Message: "missing WHERE clause", File: "update.sql", Line: 5, SQL: "UPDATE users SET active = false"},
	}

	var buf bytes.Buffer
	r := &TerminalReporter{}
	if err := r.Report(context.Background(), &buf, findings); err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "terminal_report.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden file not found -- run with -update to create: %v", err)
	}
	if !bytes.Equal(normalizeGoldenNewlines(buf.Bytes()), normalizeGoldenNewlines(want)) {
		t.Errorf("output mismatch with golden file.\nGot:\n%s\nWant:\n%s", buf.String(), string(want))
	}
}
