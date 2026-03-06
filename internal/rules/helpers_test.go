// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

func TestIsConstantTrueClause(t *testing.T) {
	tests := []struct {
		name   string
		clause string
		want   bool
	}{
		{name: "where one equals one", clause: "WHERE 1 = 1", want: true},
		{name: "where zero equals zero", clause: "WHERE 0 = 0", want: true},
		{name: "where nested true", clause: "WHERE (((TRUE)))", want: true},
		{name: "where not false", clause: "WHERE NOT FALSE", want: true},
		{name: "where double not false", clause: "WHERE NOT NOT FALSE", want: false},
		{name: "where real predicate", clause: "WHERE id = 1", want: false},
		{name: "having true", clause: "HAVING TRUE", want: true},
		{name: "where one equals two", clause: "WHERE 1 = 2", want: false},
		{name: "where false literal", clause: "WHERE FALSE", want: false},
		{name: "where column equals itself", clause: "WHERE id = id", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConstantTrueClause(tt.clause); got != tt.want {
				t.Fatalf("isConstantTrueClause(%q) = %v, want %v", tt.clause, got, tt.want)
			}
		})
	}
}

func TestNormalizePredicateForMatch(t *testing.T) {
	tests := []struct {
		name   string
		clause string
		want   string
	}{
		{name: "drops where prefix", clause: "WHERE 1 = 1", want: "1 = 1"},
		{name: "drops having prefix", clause: "HAVING TRUE", want: "true"},
		{name: "collapses whitespace", clause: "WHERE   NOT   FALSE", want: "not false"},
		{name: "unwraps parens", clause: "WHERE (((TRUE)))", want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePredicateForMatch(tt.clause); got != tt.want {
				t.Fatalf("normalizePredicateForMatch(%q) = %q, want %q", tt.clause, got, tt.want)
			}
		})
	}
}

func TestStripSQL(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		stripQuoted bool
		want        string
	}{
		{
			name:        "line comment removed",
			sql:         "SELECT 1 -- comment\nFROM t",
			stripQuoted: false,
			want:        "SELECT 1 \nFROM t",
		},
		{
			name:        "block comment removed",
			sql:         "SELECT /* a comment */ 1",
			stripQuoted: false,
			want:        "SELECT  1",
		},
		{
			name:        "nested block comment",
			sql:         "SELECT /* outer /* inner */ still outer */ 1",
			stripQuoted: false,
			want:        "SELECT  1",
		},
		{
			name:        "single quoted preserved when not stripping",
			sql:         "SELECT 'hello' FROM t",
			stripQuoted: false,
			want:        "SELECT 'hello' FROM t",
		},
		{
			name:        "single quoted stripped",
			sql:         "SELECT 'hello' FROM t",
			stripQuoted: true,
			want:        "SELECT   FROM t",
		},
		{
			name:        "escaped single quote preserved",
			sql:         "SELECT 'it''s' FROM t",
			stripQuoted: false,
			want:        "SELECT 'it''s' FROM t",
		},
		{
			name:        "escaped single quote stripped",
			sql:         "SELECT 'it''s' FROM t",
			stripQuoted: true,
			want:        "SELECT   FROM t",
		},
		{
			name:        "dollar quoted stripped",
			sql:         "SELECT $$body$$ FROM t",
			stripQuoted: true,
			want:        "SELECT   FROM t",
		},
		{
			name:        "tagged dollar quote stripped",
			sql:         "SELECT $tag$content$tag$ FROM t",
			stripQuoted: true,
			want:        "SELECT   FROM t",
		},
		{
			name:        "double quoted identifier stripped",
			sql:         `SELECT "Column" FROM t`,
			stripQuoted: true,
			want:        "SELECT   FROM t",
		},
		{
			name:        "plain sql unchanged",
			sql:         "SELECT 1 FROM t WHERE id = 5",
			stripQuoted: false,
			want:        "SELECT 1 FROM t WHERE id = 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripSQL(tt.sql, tt.stripQuoted); got != tt.want {
				t.Fatalf("stripSQL(%q, %v) = %q, want %q", tt.sql, tt.stripQuoted, got, tt.want)
			}
		})
	}
}

func TestWrappedBySingleParens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "simple pair", in: "(true)", want: true},
		{name: "nested pair", in: "((true))", want: true},
		{name: "trailing content", in: "(true) and x", want: false},
		{name: "no parens", in: "true", want: false},
		{name: "unbalanced", in: "(true", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrappedBySingleParens(tt.in); got != tt.want {
				t.Fatalf("wrappedBySingleParens(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
