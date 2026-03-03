// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

func TestNormalizeIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "simple name", input: "Users", want: "users"},
		{name: "backtick quoted", input: "`MyTable`", want: "mytable"},
		{name: "double quoted", input: `"MyTable"`, want: "mytable"},
		{name: "schema qualified", input: "public.users", want: "users"},
		{name: "quoted schema qualified", input: `"public"."Users"`, want: "users"},
		{name: "quoted identifier with dot keeps dot", input: `"User.Profile"`, want: "user.profile"},
		{name: "schema plus quoted identifier with dot", input: `public."User.Profile"`, want: "user.profile"},
		{name: "whitespace trimmed", input: "  users  ", want: "users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("normalizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeUsageColumn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "wildcard filtered", input: "*", want: ""},
		{name: "qualifier stripped", input: "table.column", want: "column"},
		{name: "quoted column", input: `"Column"`, want: "column"},
		{name: "wildcard after qualifier", input: "table.*", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeUsageColumn(tt.input)
			if got != tt.want {
				t.Errorf("normalizeUsageColumn(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
