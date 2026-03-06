// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"strings"
	"testing"
)

func TestRenderSQL(t *testing.T) {
	t.Run("plain SQL", func(t *testing.T) {
		rendered := renderSQL("SELECT * FROM users")
		if rendered.Cleaned != "SELECT * FROM users" {
			t.Fatalf("unexpected cleaned SQL: %q", rendered.Cleaned)
		}
		if rendered.SyntheticSource != "" {
			t.Fatalf("expected no synthetic source, got %q", rendered.SyntheticSource)
		}
	})

	t.Run("synthetic SQLAlchemy SQL", func(t *testing.T) {
		rendered := renderSQL(`/* valk-guard:synthetic sqlalchemy-ast */ SELECT "User"."id" FROM "User"`)
		if rendered.Cleaned != `SELECT "User"."id" FROM "User"` {
			t.Fatalf("unexpected cleaned SQL: %q", rendered.Cleaned)
		}
		if rendered.SyntheticSource != "sqlalchemy-ast" {
			t.Fatalf("expected sqlalchemy-ast, got %q", rendered.SyntheticSource)
		}
	})
}

func TestTruncateSnippet(t *testing.T) {
	if got := truncateSnippet("SELECT 1", 20); got != "SELECT 1" {
		t.Fatalf("expected unmodified snippet, got %q", got)
	}

	got := truncateSnippet("SELECT abcdefghijklmnop", 10)
	if got != "SELECT abc..." {
		t.Fatalf("unexpected truncated snippet: %q", got)
	}
}

func TestMeaningfulSQL(t *testing.T) {
	sql := strings.Join([]string{
		"",
		"-- VG004 example",
		"/* metadata */",
		"SELECT * FROM users",
		"WHERE active = true",
	}, "\n")

	got := meaningfulSQL(sql)
	want := "SELECT * FROM users\nWHERE active = true"
	if got != want {
		t.Fatalf("meaningfulSQL() = %q, want %q", got, want)
	}
}

func TestPreviewLine(t *testing.T) {
	sql := "\n   WITH active_users AS (\n  SELECT `id` FROM users\n)\n"
	got := previewLine(sql, 24)
	if got != "WITH active_users AS (" {
		t.Fatalf("unexpected preview line: %q", got)
	}
}

func TestPreviewLineSkipsLeadingComments(t *testing.T) {
	sql := strings.Join([]string{
		"-- VG004: unbounded-select",
		"/* valk metadata */",
		"SELECT * FROM users",
	}, "\n")

	got := previewLine(sql, 80)
	if got != "SELECT * FROM users" {
		t.Fatalf("expected preview to skip comments, got %q", got)
	}
}

func TestSyntheticOriginLabel(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "", want: ""},
		{source: "sqlalchemy-ast", want: "SQLAlchemy query builder"},
		{source: "goqu-ast", want: "goqu query builder"},
		{source: "custom-ast", want: "synthetic query builder"},
	}

	for _, tt := range tests {
		if got := syntheticOriginLabel(tt.source); got != tt.want {
			t.Fatalf("syntheticOriginLabel(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}
