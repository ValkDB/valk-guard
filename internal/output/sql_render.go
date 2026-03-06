// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"regexp"
	"strings"
)

const (
	terminalSnippetMaxLen  = 120
	reviewdogPreviewMaxLen = 90
)

var syntheticPrefixPattern = regexp.MustCompile(`(?is)^\s*/\*\s*valk-guard:synthetic\s+([a-z0-9_-]+)\s*\*/\s*`)

type renderedSQL struct {
	Cleaned         string
	SyntheticSource string
}

// renderSQL removes valk-guard synthetic prefixes and returns the cleaned SQL
// plus the synthetic source tag, if one was present.
func renderSQL(sql string) renderedSQL {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return renderedSQL{}
	}

	matches := syntheticPrefixPattern.FindStringSubmatch(sql)
	if len(matches) == 2 {
		return renderedSQL{
			Cleaned:         strings.TrimSpace(syntheticPrefixPattern.ReplaceAllString(sql, "")),
			SyntheticSource: strings.ToLower(strings.TrimSpace(matches[1])),
		}
	}

	return renderedSQL{Cleaned: sql}
}

// meaningfulSQL strips leading blank lines and leading standalone SQL comments
// so previews start from the first executable SQL line.
func meaningfulSQL(sql string) string {
	lines := strings.Split(sql, "\n")
	started := false
	inBlockComment := false
	out := make([]string, 0, len(lines))

nextLine:
	for _, line := range lines {
		if !started {
			trimmed := strings.TrimSpace(line)

			for {
				switch {
				case trimmed == "":
					continue nextLine
				case inBlockComment:
					end := strings.Index(trimmed, "*/")
					if end < 0 {
						continue nextLine
					}
					trimmed = strings.TrimSpace(trimmed[end+2:])
					inBlockComment = false
				case strings.HasPrefix(trimmed, "--"):
					continue nextLine
				case strings.HasPrefix(trimmed, "/*"):
					end := strings.Index(trimmed[2:], "*/")
					if end < 0 {
						inBlockComment = true
						continue nextLine
					}
					trimmed = strings.TrimSpace(trimmed[end+4:])
				default:
					started = true
					out = append(out, trimmed)
					continue nextLine
				}
			}
		} else {
			out = append(out, strings.TrimRight(line, " \t"))
		}
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

// truncateSnippet shortens a SQL snippet to maxLen characters, adding an
// ellipsis when truncation occurs.
func truncateSnippet(sql string, maxLen int) string {
	sql = meaningfulSQL(sql)
	if sql == "" {
		return ""
	}
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen] + "..."
}

// previewLine returns the first non-empty SQL line collapsed to single spaces
// and truncated for compact inline diagnostics.
func previewLine(sql string, maxLen int) string {
	for _, line := range strings.Split(meaningfulSQL(sql), "\n") {
		compact := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if compact == "" {
			continue
		}
		compact = strings.ReplaceAll(compact, "`", "'")
		if len(compact) <= maxLen {
			return compact
		}
		return compact[:maxLen] + "..."
	}
	return ""
}

// syntheticOriginLabel maps an internal synthetic source tag to a human-readable
// origin label for user-facing output.
func syntheticOriginLabel(source string) string {
	switch source {
	case "sqlalchemy-ast":
		return "SQLAlchemy query builder"
	case "goqu-ast":
		return "goqu query builder"
	case "":
		return ""
	default:
		return "synthetic query builder"
	}
}
