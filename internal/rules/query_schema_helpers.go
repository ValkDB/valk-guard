// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/schema"
)

type resolvedQueryTables struct {
	byAlias map[string]*schema.TableDef
	unique  []*schema.TableDef
}

// resolveQueryTables builds table-resolution context for query column checks.
func resolveQueryTables(snap *schema.Snapshot, parsed *postgresparser.ParsedQuery) resolvedQueryTables {
	resolved := resolvedQueryTables{
		byAlias: make(map[string]*schema.TableDef),
	}
	seen := make(map[*schema.TableDef]bool)

	for _, tbl := range parsed.Tables {
		name := normalizeIdentifier(tbl.Name)
		if name == "" {
			continue
		}
		td := matchTable(snap, name)
		if td == nil {
			continue
		}
		resolved.byAlias[name] = td
		if rawKey := normalizeIdentifier(tbl.Raw); rawKey != "" {
			resolved.byAlias[rawKey] = td
		}
		if aliasKey := normalizeIdentifier(tbl.Alias); aliasKey != "" {
			resolved.byAlias[aliasKey] = td
		}
		if !seen[td] {
			seen[td] = true
			resolved.unique = append(resolved.unique, td)
		}
	}

	return resolved
}

// resolveUsageTable resolves a column usage entry to a concrete schema table.
// Unqualified columns are only resolved when exactly one base table is known.
func resolveUsageTable(usage *postgresparser.ColumnUsage, resolved resolvedQueryTables) (*schema.TableDef, bool) {
	if alias := normalizeIdentifier(usage.TableAlias); alias != "" {
		td, ok := resolved.byAlias[alias]
		return td, ok
	}
	if len(resolved.unique) == 1 {
		return resolved.unique[0], true
	}
	return nil, false
}

// stripQuotesAndQualifier normalizes a SQL identifier by trimming whitespace,
// optionally dropping schema/table qualifiers, stripping outer identifier
// delimiters, and lowercasing. Qualifier stripping only applies to dots found
// outside quoted segments.
func stripQuotesAndQualifier(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	if idx := lastDotOutsideQuotes(name); idx >= 0 {
		name = name[idx+1:]
	}
	name = trimOuterIdentifierDelimiters(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}

func lastDotOutsideQuotes(s string) int {
	var quote byte
	last := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch != quote {
				continue
			}
			if quote == '"' && i+1 < len(s) && s[i+1] == '"' {
				i++
				continue
			}
			if quote == '`' && i+1 < len(s) && s[i+1] == '`' {
				i++
				continue
			}
			quote = 0
			continue
		}
		if ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '.' {
			last = i
		}
	}
	return last
}

func trimOuterIdentifierDelimiters(name string) string {
	if len(name) < 2 {
		return name
	}
	first := name[0]
	last := name[len(name)-1]
	if (first == '"' && last == '"') || (first == '`' && last == '`') {
		return name[1 : len(name)-1]
	}
	return name
}

// normalizeIdentifier canonicalizes SQL identifiers used for table/alias
// resolution.
func normalizeIdentifier(name string) string {
	return stripQuotesAndQualifier(name)
}

// normalizeUsageColumn converts parser column text into a lowercase key used
// to look up columns in schema.TableDef.Columns. Wildcards (*) are filtered out.
func normalizeUsageColumn(col string) string {
	result := stripQuotesAndQualifier(col)
	if result == "*" {
		return ""
	}
	return result
}
