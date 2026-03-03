// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import "testing"

func TestParseDirectives(t *testing.T) {
	tests := []struct {
		name           string
		lines          []string
		wantCount      int
		wantDirectives []Directive // expected directives (line and ruleIDs)
	}{
		{
			name: "SQL comments with single and multiple rule IDs",
			lines: []string{
				"-- valk-guard:disable VG001",
				"SELECT * FROM users;",
				"",
				"-- valk-guard:disable VG002,VG003",
				"SELECT id FROM orders;",
			},
			wantCount: 2,
			wantDirectives: []Directive{
				{Line: 1, RuleIDs: []string{"VG001"}},
				{Line: 4, RuleIDs: []string{"VG002", "VG003"}},
			},
		},
		{
			name: "Go comment style",
			lines: []string{
				"// valk-guard:disable VG001",
				`db.Query("SELECT * FROM users")`,
			},
			wantCount: 1,
			wantDirectives: []Directive{
				{Line: 1, RuleIDs: []string{"VG001"}},
			},
		},
		{
			name: "no directives in plain SQL",
			lines: []string{
				"SELECT * FROM users;",
				"SELECT id FROM orders;",
			},
			wantCount: 0,
		},
		{
			name: "disable all (no rule IDs)",
			lines: []string{
				"-- valk-guard:disable",
				"SELECT * FROM users;",
			},
			wantCount: 1,
			wantDirectives: []Directive{
				{Line: 1, RuleIDs: []string{DisableAll}},
			},
		},
		{
			name: "Python hash comment",
			lines: []string{
				"# valk-guard:disable VG001",
				`result = session.execute(text("SELECT * FROM users"))`,
			},
			wantCount: 1,
			wantDirectives: []Directive{
				{Line: 1, RuleIDs: []string{"VG001"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directives := ParseDirectives(tt.lines)

			if len(directives) != tt.wantCount {
				t.Fatalf("expected %d directives, got %d", tt.wantCount, len(directives))
			}

			for i, want := range tt.wantDirectives {
				got := directives[i]
				if got.Line != want.Line {
					t.Errorf("directive %d: expected line %d, got %d", i, want.Line, got.Line)
				}
				if len(got.RuleIDs) != len(want.RuleIDs) {
					t.Errorf("directive %d: expected %d rule IDs, got %d (%v)", i, len(want.RuleIDs), len(got.RuleIDs), got.RuleIDs)
					continue
				}
				for j, wantID := range want.RuleIDs {
					if got.RuleIDs[j] != wantID {
						t.Errorf("directive %d, rule %d: expected %s, got %s", i, j, wantID, got.RuleIDs[j])
					}
				}
			}
		})
	}
}

func TestIsDisabled(t *testing.T) {
	tests := []struct {
		name     string
		ruleID   string
		disabled []string
		want     bool
	}{
		{
			name:     "disabled rule is detected",
			ruleID:   "VG001",
			disabled: []string{"VG001", "VG003"},
			want:     true,
		},
		{
			name:     "non-disabled rule is not detected",
			ruleID:   "VG002",
			disabled: []string{"VG001", "VG003"},
			want:     false,
		},
		{
			name:     "another disabled rule is detected",
			ruleID:   "VG003",
			disabled: []string{"VG001", "VG003"},
			want:     true,
		},
		{
			name:     "DisableAll disables any rule",
			ruleID:   "VG999",
			disabled: []string{DisableAll},
			want:     true,
		},
		{
			name:     "empty disabled list",
			ruleID:   "VG001",
			disabled: []string{},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDisabled(tt.ruleID, tt.disabled)
			if got != tt.want {
				t.Errorf("IsDisabled(%q, %v) = %v, want %v", tt.ruleID, tt.disabled, got, tt.want)
			}
		})
	}
}
