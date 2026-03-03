package rules

import "testing"

// TestStripSQL validates stripSQL with both stripQuoted=false and stripQuoted=true.
func TestStripSQL(t *testing.T) {
	t.Run("stripQuoted=false (comments only)", func(t *testing.T) {
		tests := []struct {
			name string
			sql  string
			want string
		}{
			{
				// No comments at all: input is returned unchanged.
				name: "no comments unchanged",
				sql:  "SELECT id, name FROM users WHERE id = 1",
				want: "SELECT id, name FROM users WHERE id = 1",
			},
			{
				// Single-line comment starting with -- is removed up to (but not
				// including) the newline.
				name: "single-line comment stripped",
				sql:  "SELECT 1 -- this is a comment",
				want: "SELECT 1 ",
			},
			{
				// Block comment delimited by /* */ is removed entirely.
				name: "block comment stripped",
				sql:  "SELECT /* hidden */ 1",
				want: "SELECT  1",
			},
			{
				// Nested block comments (/* outer /* inner */ end */) are handled
				// correctly by tracking depth; all content is removed.
				name: "nested block comment stripped",
				sql:  "SELECT /* outer /* inner */ still outer */ 1",
				want: "SELECT  1",
			},
			{
				// String literals must NOT be touched when stripQuoted is false.
				name: "string literal preserved",
				sql:  "SELECT 'hello world' FROM t",
				want: "SELECT 'hello world' FROM t",
			},
			{
				// A string that looks like a comment must not be stripped.
				name: "comment-like text inside string literal preserved",
				sql:  "SELECT '-- not a comment' FROM t",
				want: "SELECT '-- not a comment' FROM t",
			},
			{
				// A string that looks like a block comment must not be stripped.
				name: "block-comment-like text inside string preserved",
				sql:  "SELECT '/* also not a comment */' FROM t",
				want: "SELECT '/* also not a comment */' FROM t",
			},
			{
				// Mixed: real comment stripped, string literal kept intact.
				name: "mixed comment and string literal",
				sql:  "SELECT '/* ok */' -- line comment\nFROM t /* block */",
				want: "SELECT '/* ok */' \nFROM t ",
			},
			{
				// A block comment that spans multiple lines must preserve the
				// newline characters so that reported line numbers remain correct.
				name: "multi-line block comment preserves newlines",
				sql:  "SELECT\n/* line1\nline2\n*/\n1",
				want: "SELECT\n\n\n\n1",
			},
			{
				// A single-line comment must not swallow the trailing newline; the
				// newline becomes the first character of the next iteration so that
				// subsequent lines keep correct line numbers.
				name: "line comment does not consume trailing newline",
				sql:  "SELECT 1 -- comment\nFROM t",
				want: "SELECT 1 \nFROM t",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := stripSQL(tt.sql, false)
				if got != tt.want {
					t.Errorf("stripSQL(%q, false)\n  got  %q\n  want %q", tt.sql, got, tt.want)
				}
			})
		}
	})

	t.Run("stripQuoted=true (comments + quoted segments)", func(t *testing.T) {
		tests := []struct {
			name string
			sql  string
			want string
		}{
			{
				// Empty input produces empty output.
				name: "empty input",
				sql:  "",
				want: "",
			},
			{
				// Input that is only a comment produces an empty string (the
				// comment is stripped and nothing else is written).
				name: "input with only a line comment",
				sql:  "-- just a comment",
				want: "",
			},
			{
				// Input that is only a block comment produces an empty string.
				name: "input with only a block comment",
				sql:  "/* just a block comment */",
				want: "",
			},
			{
				// Single-quoted string literal is replaced by a single space.
				name: "single-quoted string replaced with space",
				sql:  "SELECT 'hello' FROM t",
				want: "SELECT   FROM t",
			},
			{
				// Double-quoted identifier is replaced by a single space.
				name: "double-quoted identifier replaced with space",
				sql:  `SELECT "col_name" FROM t`,
				want: "SELECT   FROM t",
			},
			{
				// Simple $$ dollar-quoted string is replaced by a single space.
				name: "dollar-quoted string ($$) replaced with space",
				sql:  "SELECT $$body$$ FROM t",
				want: "SELECT   FROM t",
			},
			{
				// Tagged dollar-quoted string ($tag$...$tag$) is replaced by a
				// single space.
				name: "tagged dollar-quoted string replaced with space",
				sql:  "SELECT $mytag$body$mytag$ FROM t",
				want: "SELECT   FROM t",
			},
			{
				// A doubled single quote inside a string (escaped quote) must be
				// consumed as part of the literal, not treated as the end of the
				// string followed by a new string.
				name: "escaped single quote inside string handled correctly",
				sql:  "SELECT 'it''s fine' FROM t",
				want: "SELECT   FROM t",
			},
			{
				// A doubled double-quote inside a double-quoted identifier (escaped
				// quote) must be consumed as part of the identifier.
				name: "escaped double quote inside identifier handled correctly",
				sql:  `SELECT "col""name" FROM t`,
				want: "SELECT   FROM t",
			},
			{
				// Both a comment and a quoted segment appear: the comment is
				// stripped and the string is replaced with a space.
				name: "mixed: comment stripped and string replaced",
				sql:  "SELECT 'value' -- pick a value\nFROM t",
				want: "SELECT   \nFROM t",
			},
			{
				// The primary use-case: FOR UPDATE inside a string literal must
				// NOT trigger the FOR UPDATE locking-clause rule.
				name: "FOR UPDATE inside string literal is stripped",
				sql:  "SELECT 'FOR UPDATE' FROM t",
				want: "SELECT   FROM t",
			},
			{
				// FOR UPDATE outside any quoting context must be preserved so that
				// the rule can detect it.
				name: "FOR UPDATE outside quotes is preserved",
				sql:  "SELECT * FROM t FOR UPDATE",
				want: "SELECT * FROM t FOR UPDATE",
			},
			{
				// A block comment that spans multiple lines must still preserve
				// newline characters inside the comment body.
				name: "newlines inside block comment preserved",
				sql:  "SELECT\n/*\ncomment\n*/\n1",
				want: "SELECT\n\n\n\n1",
			},
			{
				// Newlines inside a single-quoted string are preserved (the
				// implementation emits '\n' for each newline encountered inside the
				// quoted body, then writes the single replacement space after the
				// closing quote).
				name: "newlines inside single-quoted string preserved",
				sql:  "SELECT 'line1\nline2' FROM t",
				want: "SELECT \n  FROM t",
			},
			{
				// Newlines inside a double-quoted identifier are preserved.
				// The implementation emits '\n' during body scanning, then writes
				// the replacement space after the closing '"'.
				name: "newlines inside double-quoted identifier preserved",
				sql:  "SELECT \"col\nname\" FROM t",
				want: "SELECT \n  FROM t",
			},
			{
				// A $$ dollar-quoted string whose body contains newlines must
				// preserve those newlines.  The implementation emits '\n' during
				// body scanning, then writes the replacement space after the
				// closing tag.
				name: "newlines inside dollar-quoted string preserved",
				sql:  "SELECT $$line1\nline2$$ FROM t",
				want: "SELECT \n  FROM t",
			},
			{
				// FOR UPDATE appearing in a comment must also not trigger; the
				// comment is stripped before the regex check.
				name: "FOR UPDATE inside line comment is stripped",
				sql:  "SELECT * FROM t -- FOR UPDATE",
				want: "SELECT * FROM t ",
			},
			{
				// FOR UPDATE inside a block comment is stripped.
				name: "FOR UPDATE inside block comment is stripped",
				sql:  "SELECT * FROM t /* FOR UPDATE */",
				want: "SELECT * FROM t ",
			},
			{
				// No modifications when no comments, strings, or identifiers.
				name: "plain SQL without any quoting unchanged",
				sql:  "DELETE FROM t WHERE id = 42",
				want: "DELETE FROM t WHERE id = 42",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := stripSQL(tt.sql, true)
				if got != tt.want {
					t.Errorf("stripSQL(%q, true)\n  got  %q\n  want %q", tt.sql, got, tt.want)
				}
			})
		}
	})
}

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
