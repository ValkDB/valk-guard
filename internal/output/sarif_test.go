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

func TestSARIFReporterEmpty(t *testing.T) {
	r := &SARIFReporter{}
	var buf bytes.Buffer

	if err := r.Report(context.Background(), &buf, nil); err != nil {
		t.Fatalf("report error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["version"] != "2.1.0" {
		t.Errorf("expected SARIF version 2.1.0, got %v", result["version"])
	}

	if result["$schema"] != sarifSchema {
		t.Errorf("expected SARIF schema URI, got %v", result["$schema"])
	}

	runs, ok := result["runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatal("expected 1 run")
	}

	run := runs[0].(map[string]interface{})
	results := run["results"].([]interface{})
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSARIFReporterWithFindings(t *testing.T) {
	r := &SARIFReporter{}
	var buf bytes.Buffer

	findings := []rules.Finding{
		{
			RuleID:   "VG001",
			Severity: rules.SeverityError,
			Message:  "avoid SELECT *",
			File:     "test.sql",
			Line:     10,
			Column:   1,
		},
	}

	if err := r.Report(context.Background(), &buf, findings); err != nil {
		t.Fatalf("report error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	runs := result["runs"].([]interface{})
	run := runs[0].(map[string]interface{})

	// Verify tool metadata.
	tool := run["tool"].(map[string]interface{})
	driver := tool["driver"].(map[string]interface{})
	if driver["name"] != "valk-guard" {
		t.Errorf("expected tool name 'valk-guard', got %v", driver["name"])
	}

	// Verify rules.
	ruleDescs := driver["rules"].([]interface{})
	if len(ruleDescs) != 1 {
		t.Errorf("expected 1 rule descriptor, got %d", len(ruleDescs))
	}

	// Verify fullDescription and helpUri on the rule descriptor.
	ruleDesc := ruleDescs[0].(map[string]interface{})
	sd, ok := ruleDesc["shortDescription"].(map[string]interface{})
	if !ok || sd["text"] != "Detects SELECT * projections." {
		t.Errorf("expected shortDescription.text from rule metadata, got %v", ruleDesc["shortDescription"])
	}
	if fd, ok := ruleDesc["fullDescription"].(map[string]interface{}); !ok || fd["text"] == "" {
		t.Errorf("expected non-empty fullDescription.text, got %v", ruleDesc["fullDescription"])
	}
	if dc, ok := ruleDesc["defaultConfiguration"].(map[string]interface{}); !ok || dc["level"] != "warning" {
		t.Errorf("expected defaultConfiguration.level warning, got %v", ruleDesc["defaultConfiguration"])
	}
	if ruleDesc["helpUri"] != sarifRulesHelpURI {
		t.Errorf("expected helpUri %q, got %v", sarifRulesHelpURI, ruleDesc["helpUri"])
	}

	// Verify results.
	results := run["results"].([]interface{})
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	res := results[0].(map[string]interface{})
	if res["ruleId"] != "VG001" {
		t.Errorf("expected ruleId VG001, got %v", res["ruleId"])
	}
	if res["level"] != "error" {
		t.Errorf("expected level 'error', got %v", res["level"])
	}
}

func TestSARIFSeverityMapping(t *testing.T) {
	tests := []struct {
		input    rules.Severity
		expected string
	}{
		{rules.SeverityError, "error"},
		{rules.SeverityWarning, "warning"},
		{rules.SeverityInfo, "note"},
	}

	for _, tt := range tests {
		got := severityToSARIF(tt.input)
		if got != tt.expected {
			t.Errorf("severityToSARIF(%s): expected %s, got %s", tt.input, tt.expected, got)
		}
	}
}
