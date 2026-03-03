// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"errors"
	"testing"
)

func TestParseStatementEmpty(t *testing.T) {
	parsed, err := ParseStatement("")
	if err != nil {
		t.Fatalf("expected nil error for empty SQL, got %v", err)
	}
	if parsed != nil {
		t.Fatalf("expected nil parsed result for empty SQL, got %#v", parsed)
	}
}

func TestParseStatementValidSQL(t *testing.T) {
	parsed, err := ParseStatement("SELECT 1")
	if err != nil {
		t.Fatalf("expected parse success, got error %v", err)
	}
	if parsed == nil {
		t.Fatal("expected parsed query, got nil")
	}
}

func TestParseStatementInvalidSQL(t *testing.T) {
	parsed, err := ParseStatement("SELECT FROM")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !errors.Is(err, ErrParserFailure) {
		t.Fatalf("expected ErrParserFailure, got %v", err)
	}
	if parsed != nil {
		t.Fatalf("expected nil parsed query on error, got %#v", parsed)
	}
}
