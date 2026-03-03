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
		nameKey := normalizeIdentifier(tbl.Name)
		if nameKey != "" {
			resolved.byAlias[nameKey] = td
		}
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
func resolveUsageTable(usage postgresparser.ColumnUsage, resolved resolvedQueryTables) (*schema.TableDef, bool) {
	if alias := normalizeIdentifier(usage.TableAlias); alias != "" {
		td, ok := resolved.byAlias[alias]
		return td, ok
	}
	if len(resolved.unique) == 1 {
		return resolved.unique[0], true
	}
	return nil, false
}

// normalizeUsageColumn converts parser column text into a lowercase key used to
// look up columns in schema.TableDef.Columns.
func normalizeUsageColumn(col string) string {
	col = strings.TrimSpace(col)
	col = strings.Trim(col, "\"`")
	if col == "" || col == "*" {
		return ""
	}
	if idx := strings.LastIndex(col, "."); idx >= 0 {
		col = col[idx+1:]
	}
	col = strings.Trim(col, "\"`")
	if col == "" || col == "*" {
		return ""
	}
	return strings.ToLower(col)
}

// normalizeIdentifier canonicalizes SQL identifiers used for table/alias
// resolution: trim whitespace/quotes, drop qualifiers, and lowercase.
func normalizeIdentifier(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "\"`")
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.Trim(name, "\"`")
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}
