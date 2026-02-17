package rules

import "testing"

// TestStripSQLComments validates the comment-stripping helper.
func TestStripSQLComments(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "no comments",
			sql:  "SELECT 1",
			want: "SELECT 1",
		},
		{
			name: "line comment stripped",
			sql:  "SELECT 1 -- a comment",
			want: "SELECT 1 ",
		},
		{
			name: "block comment stripped",
			sql:  "SELECT /* hidden */ 1",
			want: "SELECT  1",
		},
		{
			name: "nested block comment stripped",
			sql:  "SELECT /* outer /* inner */ end */ 1",
			want: "SELECT  1",
		},
		{
			name: "string not stripped",
			sql:  "SELECT '-- not a comment' FROM t",
			want: "SELECT '-- not a comment' FROM t",
		},
		{
			name: "mixed comments and strings",
			sql:  "SELECT '/* ok */' -- line comment\nFROM t /* block */",
			want: "SELECT '/* ok */' \nFROM t ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSQLComments(tt.sql)
			if got != tt.want {
				t.Errorf("stripSQLComments(%q)\n  got  %q\n  want %q", tt.sql, got, tt.want)
			}
		})
	}
}
