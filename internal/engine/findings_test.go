// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestApplyStatementRange(t *testing.T) {
	findings := []rules.Finding{
		{
			RuleID:  "VG106",
			File:    "complex_queries.sql",
			Line:    21,
			Column:  1,
			Message: "unknown filter column",
		},
	}

	applyStatementRange(findings, &scanner.SQLStatement{
		File:      "complex_queries.sql",
		Line:      21,
		Column:    5,
		EndLine:   26,
		EndColumn: 9,
	})

	if findings[0].Column != 5 {
		t.Fatalf("expected statement start column to propagate, got %d", findings[0].Column)
	}
	if findings[0].EndLine != 26 || findings[0].EndColumn != 9 {
		t.Fatalf("expected statement end range to propagate, got end=%d:%d", findings[0].EndLine, findings[0].EndColumn)
	}
}
