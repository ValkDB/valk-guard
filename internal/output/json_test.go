package output

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
)

func TestJSONReporter(t *testing.T) {
	tests := []struct {
		name          string
		findings      []rules.Finding
		wantCount     int
		wantFirstRule string
	}{
		{
			name:      "empty findings produces empty JSON array",
			findings:  nil,
			wantCount: 0,
		},
		{
			name: "multiple findings are serialized correctly",
			findings: []rules.Finding{
				{RuleID: "VG001", Severity: rules.SeverityError, Message: "avoid SELECT *", File: "test.sql", Line: 10},
				{RuleID: "VG002", Severity: rules.SeverityWarning, Message: "missing WHERE", File: "test.sql", Line: 20},
			},
			wantCount:     2,
			wantFirstRule: "VG001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &JSONReporter{}
			var buf bytes.Buffer

			if err := r.Report(context.Background(), &buf, tt.findings); err != nil {
				t.Fatalf("report error: %v", err)
			}

			var result []rules.Finding
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("expected %d findings, got %d", tt.wantCount, len(result))
			}

			if tt.wantFirstRule != "" && len(result) > 0 {
				if result[0].RuleID != tt.wantFirstRule {
					t.Errorf("expected first rule %s, got %s", tt.wantFirstRule, result[0].RuleID)
				}
			}
		})
	}
}

func TestJSONReportGolden(t *testing.T) {
	findings := []rules.Finding{
		{RuleID: "VG001", Severity: rules.SeverityWarning, Message: "avoid SELECT *", File: "query.sql", Line: 10},
		{RuleID: "VG002", Severity: rules.SeverityError, Message: "missing WHERE clause", File: "update.sql", Line: 5},
	}

	var buf bytes.Buffer
	r := &JSONReporter{}
	if err := r.Report(context.Background(), &buf, findings); err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "json_report.golden")
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
