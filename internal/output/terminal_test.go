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
	}

	tests := []struct {
		name         string
		noColor      bool
		findings     []rules.Finding
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "no findings",
			noColor:      false,
			findings:     nil,
			wantContains: []string{"0 findings"},
		},
		{
			name:     "single finding with correct format",
			noColor:  true,
			findings: []rules.Finding{errorFinding},
			wantContains: []string{
				"test.sql:10",
				"[VG001]",
				"1 finding",
			},
			wantAbsent: []string{"1 findings"},
		},
		{
			name:    "no-color mode omits ANSI codes",
			noColor: true,
			findings: []rules.Finding{
				{RuleID: "VG001", Severity: rules.SeverityError, Message: "test", File: "test.sql", Line: 1},
			},
			wantAbsent: []string{"\033["},
		},
		{
			name:    "color mode includes ANSI codes",
			noColor: false,
			findings: []rules.Finding{
				{RuleID: "VG001", Severity: rules.SeverityError, Message: "test", File: "test.sql", Line: 1},
			},
			wantContains: []string{"\033[31m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")

			r := &TerminalReporter{NoColor: tt.noColor}
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

func TestTerminalReporterNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	r := &TerminalReporter{NoColor: false}
	var buf bytes.Buffer

	findings := []rules.Finding{
		{RuleID: "VG001", Severity: rules.SeverityError, Message: "test", File: "test.sql", Line: 1},
	}

	if err := r.Report(context.Background(), &buf, findings); err != nil {
		t.Fatalf("report error: %v", err)
	}

	if strings.Contains(buf.String(), "\033[") {
		t.Error("expected no ANSI escape codes when NO_COLOR env is set")
	}
}

func TestTerminalReportGolden(t *testing.T) {
	findings := []rules.Finding{
		{RuleID: "VG001", Severity: rules.SeverityWarning, Message: "avoid SELECT *", File: "query.sql", Line: 10},
		{RuleID: "VG002", Severity: rules.SeverityError, Message: "missing WHERE clause", File: "update.sql", Line: 5},
	}

	var buf bytes.Buffer
	r := &TerminalReporter{NoColor: true}
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
