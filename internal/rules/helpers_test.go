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
