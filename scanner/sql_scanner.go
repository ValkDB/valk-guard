package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RawSQLScanner finds .sql files and splits them into individual statements.
type RawSQLScanner struct{}

// Scan walks the given paths, finds .sql files, and splits them on semicolons.
func (s *RawSQLScanner) Scan(paths []string) ([]SQLStatement, error) {
	var results []SQLStatement

	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".sql" {
				return nil
			}

			stmts, err := scanSQLFile(path)
			if err != nil {
				return err
			}
			results = append(results, stmts...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// scanSQLFile reads a SQL file and splits it into individual statements using
// a character-level scanner that correctly handles semicolons inside:
//   - single-quoted strings ('...'), including escaped quotes (”)
//   - double-quoted identifiers ("...")
//   - dollar-quoted strings ($$...$$)
//   - single-line comments (-- ...)
//   - block comments (/* ... */)
func scanSQLFile(path string) ([]SQLStatement, error) {
	data, err := os.ReadFile(path) //nolint:gosec // scanning user-provided source paths
	if err != nil {
		return nil, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	directives := ParseDirectives(lines)

	var results []SQLStatement
	var current strings.Builder
	startLine := 0
	line := 1

	i := 0
	for i < len(content) {
		ch := content[i]

		// Track newlines for line counting.
		if ch == '\n' {
			current.WriteByte(ch)
			line++
			i++
			continue
		}

		// Single-line comment: -- until end of line.
		if ch == '-' && i+1 < len(content) && content[i+1] == '-' {
			for i < len(content) && content[i] != '\n' {
				current.WriteByte(content[i])
				i++
			}
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && i+1 < len(content) && content[i+1] == '*' {
			current.WriteByte(content[i])
			i++
			current.WriteByte(content[i])
			i++
			for i < len(content) {
				if content[i] == '\n' {
					line++
				}
				if content[i] == '*' && i+1 < len(content) && content[i+1] == '/' {
					current.WriteByte(content[i])
					i++
					current.WriteByte(content[i])
					i++
					break
				}
				current.WriteByte(content[i])
				i++
			}
			continue
		}

		// Dollar-quoted string: $$...$$ (or $tag$...$tag$).
		if ch == '$' {
			// Try to find the end of the dollar-quote tag.
			tag := scanDollarTag(content, i)
			if tag != "" {
				// Write the opening tag.
				current.WriteString(tag)
				i += len(tag)
				if startLine == 0 {
					startLine = line
				}
				// Scan until we find the closing tag.
				for i < len(content) {
					if content[i] == '\n' {
						line++
					}
					if content[i] == '$' && strings.HasPrefix(content[i:], tag) {
						current.WriteString(tag)
						i += len(tag)
						break
					}
					current.WriteByte(content[i])
					i++
				}
				continue
			}
		}

		// Single-quoted string: '...' with '' as escape.
		if ch == '\'' {
			current.WriteByte(ch)
			i++
			if startLine == 0 {
				startLine = line
			}
			for i < len(content) {
				if content[i] == '\n' {
					line++
				}
				if content[i] == '\'' {
					current.WriteByte(content[i])
					i++
					// Check for escaped quote ('').
					if i < len(content) && content[i] == '\'' {
						current.WriteByte(content[i])
						i++
						continue
					}
					break
				}
				current.WriteByte(content[i])
				i++
			}
			continue
		}

		// Double-quoted identifier: "..."
		if ch == '"' {
			current.WriteByte(ch)
			i++
			if startLine == 0 {
				startLine = line
			}
			for i < len(content) {
				if content[i] == '\n' {
					line++
				}
				if content[i] == '"' {
					current.WriteByte(content[i])
					i++
					break
				}
				current.WriteByte(content[i])
				i++
			}
			continue
		}

		// Semicolon outside all quoted/comment contexts: end of statement.
		if ch == ';' {
			sql := strings.TrimSpace(current.String())
			if sql != "" {
				if startLine == 0 {
					startLine = line
				}
				results = append(results, SQLStatement{
					SQL:      sql,
					File:     path,
					Line:     startLine,
					Disabled: disabledRulesForLine(directives, startLine),
				})
			}
			current.Reset()
			startLine = 0
			i++
			continue
		}

		// Regular character.
		if startLine == 0 && ch != ' ' && ch != '\t' && ch != '\r' {
			startLine = line
		}
		current.WriteByte(ch)
		i++
	}

	// Handle trailing statement without semicolon.
	if trailing := strings.TrimSpace(current.String()); trailing != "" {
		if startLine == 0 {
			startLine = line
		}
		results = append(results, SQLStatement{
			SQL:      trailing,
			File:     path,
			Line:     startLine,
			Disabled: disabledRulesForLine(directives, startLine),
		})
	}

	return results, nil
}

// scanDollarTag checks if content[pos] starts a valid dollar-quote tag like
// $$ or $tag$. Returns the full tag string or "" if not a dollar-quote tag.
func scanDollarTag(content string, pos int) string {
	if pos >= len(content) || content[pos] != '$' {
		return ""
	}
	// Scan for $identifier$ or just $$.
	j := pos + 1
	for j < len(content) {
		ch := content[j]
		if ch == '$' {
			return content[pos : j+1]
		}
		// Dollar-quote tag identifiers must be letters, digits, or underscores,
		// and must not start with a digit.
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || (ch >= '0' && ch <= '9' && j > pos+1) {
			j++
			continue
		}
		break
	}
	return ""
}

// disabledRulesForLine returns the rule IDs disabled by directives on
// the given line or the immediately preceding line.
func disabledRulesForLine(directives []Directive, line int) []string {
	var disabled []string
	for _, d := range directives {
		if d.Line == line || d.Line == line-1 {
			disabled = append(disabled, d.RuleIDs...)
		}
	}
	return disabled
}
