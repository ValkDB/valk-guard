// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package gomodel

import (
	"context"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// MappingMode controls how Go model extraction resolves table and column
// mappings when explicit metadata is missing.
type MappingMode string

const (
	MappingModeStrict     MappingMode = "strict"
	MappingModeBalanced   MappingMode = "balanced"
	MappingModePermissive MappingMode = "permissive"
)

// FieldContext describes one Go struct field candidate presented to a column
// mapping provider.
type FieldContext struct {
	Name     string
	Exported bool
	Tag      reflect.StructTag
	Type     string
	Line     int
}

// ColumnResolution is a provider-produced column mapping.
type ColumnResolution struct {
	Column string
	Kind   schema.MappingKind
	Source string
}

// ColumnMappingProvider resolves a DB column mapping for a Go struct field.
type ColumnMappingProvider interface {
	Name() string
	ResolveColumn(field FieldContext, mode MappingMode) (ColumnResolution, bool)
}

// TableContext describes one Go type candidate presented to a table mapping
// provider.
type TableContext struct {
	TypeName         string
	MethodTableNames map[string]string
}

// TableResolution is a provider-produced table mapping.
type TableResolution struct {
	Table    string
	Kind     schema.MappingKind
	Explicit bool
	Source   string
}

// TableMappingProvider resolves a DB table mapping for a Go struct type.
type TableMappingProvider interface {
	Name() string
	ResolveTable(ctx TableContext, mode MappingMode) (TableResolution, bool)
}

// Extractor extracts model definitions from Go source files using a provider
// pipeline for table/column mapping.
type Extractor struct {
	Mode            MappingMode
	ColumnProviders []ColumnMappingProvider
	TableProviders  []TableMappingProvider
}

// ExtractModels walks the given paths for Go source files and returns a
// ModelDef for every struct that yields at least one mapped column.
func (e *Extractor) ExtractModels(ctx context.Context, paths []string) ([]schema.ModelDef, error) {
	var models []schema.ModelDef

	mode := normalizeMode(e.Mode)
	columnProviders := e.resolvedColumnProviders()
	tableProviders := e.resolvedTableProviders()

	err := scanner.WalkGoFiles(ctx, paths, func(path string, fset *token.FileSet, file *ast.File, _ []byte) error {
		methodTables := collectTableNameMethods(file)

		ast.Inspect(file, func(n ast.Node) bool {
			if ctx.Err() != nil {
				return false
			}

			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}

			table := resolveTableMapping(ts.Name.Name, methodTables, mode, tableProviders)
			cols := extractColumnsFromStruct(st, fset, mode, columnProviders)
			if len(cols) == 0 {
				return true
			}

			models = append(models, schema.ModelDef{
				Table:              table.Table,
				TableExplicit:      table.Explicit,
				TableMappingKind:   table.Kind,
				TableMappingSource: table.Source,
				Source:             schema.ModelSourceGo,
				Columns:            cols,
				File:               path,
				Line:               fset.Position(ts.Pos()).Line,
			})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (e *Extractor) resolvedColumnProviders() []ColumnMappingProvider {
	if len(e.ColumnProviders) > 0 {
		return e.ColumnProviders
	}
	return []ColumnMappingProvider{
		&DBTagProvider{},
		&GormTagProvider{},
		&InferredNameProvider{},
	}
}

func (e *Extractor) resolvedTableProviders() []TableMappingProvider {
	if len(e.TableProviders) > 0 {
		return e.TableProviders
	}
	return []TableMappingProvider{
		&TableNameMethodProvider{},
		&InferredTableNameProvider{},
	}
}

func normalizeMode(mode MappingMode) MappingMode {
	candidate := MappingMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch candidate {
	case MappingModeStrict, MappingModeBalanced, MappingModePermissive:
		return candidate
	default:
		return MappingModeStrict
	}
}

// extractColumnsFromStruct extracts ModelColumn values from struct fields using
// the configured provider pipeline.
func extractColumnsFromStruct(
	st *ast.StructType,
	fset *token.FileSet,
	mode MappingMode,
	providers []ColumnMappingProvider,
) []schema.ModelColumn {
	var cols []schema.ModelColumn

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// Embedded fields are intentionally skipped for now.
			continue
		}

		fieldType := normalizeGoType(field.Type)
		tag := parseFieldTag(field.Tag)

		for _, ident := range field.Names {
			if ident == nil {
				continue
			}

			ctx := FieldContext{
				Name:     ident.Name,
				Exported: ast.IsExported(ident.Name),
				Tag:      tag,
				Type:     fieldType,
				Line:     fset.Position(ident.Pos()).Line,
			}

			resolved, ok := resolveColumnMapping(ctx, mode, providers)
			if !ok {
				continue
			}

			cols = append(cols, schema.ModelColumn{
				Name:          resolved.Column,
				Field:         ctx.Name,
				Type:          fieldType,
				Line:          ctx.Line,
				MappingKind:   resolved.Kind,
				MappingSource: resolved.Source,
			})
		}
	}

	return cols
}

func resolveColumnMapping(field FieldContext, mode MappingMode, providers []ColumnMappingProvider) (ColumnResolution, bool) {
	for _, provider := range providers {
		resolved, ok := provider.ResolveColumn(field, mode)
		if !ok {
			continue
		}

		resolved.Column = strings.TrimSpace(resolved.Column)
		if resolved.Column == "" || resolved.Column == "-" {
			continue
		}
		if resolved.Source == "" {
			resolved.Source = provider.Name()
		}
		if resolved.Kind == "" {
			resolved.Kind = schema.MappingKindExplicit
		}
		return resolved, true
	}
	return ColumnResolution{}, false
}

func parseFieldTag(tagLit *ast.BasicLit) reflect.StructTag {
	if tagLit == nil {
		return ""
	}
	unquoted, err := strconv.Unquote(tagLit.Value)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted)
}

func resolveTableMapping(
	typeName string,
	methodTableNames map[string]string,
	mode MappingMode,
	providers []TableMappingProvider,
) TableResolution {
	ctx := TableContext{TypeName: typeName, MethodTableNames: methodTableNames}
	for _, provider := range providers {
		resolved, ok := provider.ResolveTable(ctx, mode)
		if !ok {
			continue
		}
		resolved.Table = strings.TrimSpace(resolved.Table)
		if resolved.Table == "" {
			continue
		}
		if resolved.Kind == "" {
			if resolved.Explicit {
				resolved.Kind = schema.MappingKindExplicit
			} else {
				resolved.Kind = schema.MappingKindInferred
			}
		}
		if resolved.Source == "" {
			resolved.Source = provider.Name()
		}
		return resolved
	}

	return TableResolution{
		Table:    strings.ToLower(typeName),
		Kind:     schema.MappingKindInferred,
		Explicit: false,
		Source:   "type_name",
	}
}

func collectTableNameMethods(file *ast.File) map[string]string {
	result := make(map[string]string)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "TableName" {
			continue
		}

		recvType := receiverTypeName(fn.Recv)
		if recvType == "" {
			continue
		}

		tableName, ok := returnedStringLiteral(fn)
		if !ok {
			continue
		}

		result[recvType] = tableName
	}

	return result
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return receiverExprTypeName(recv.List[0].Type)
}

func receiverExprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverExprTypeName(t.X)
	case *ast.IndexExpr:
		return receiverExprTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverExprTypeName(t.X)
	default:
		return ""
	}
}

func returnedStringLiteral(fn *ast.FuncDecl) (string, bool) {
	if fn == nil || fn.Body == nil {
		return "", false
	}
	for _, stmt := range fn.Body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		lit, ok := ret.Results[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		return value, true
	}
	return "", false
}

// DBTagProvider resolves mappings from `db:"column"` struct tags.
type DBTagProvider struct{}

func (p *DBTagProvider) Name() string { return "db_tag" }

func (p *DBTagProvider) ResolveColumn(field FieldContext, _ MappingMode) (ColumnResolution, bool) {
	value, ok := field.Tag.Lookup("db")
	if !ok {
		return ColumnResolution{}, false
	}
	column := primaryTagValue(value, ',')
	if column == "" || column == "-" {
		return ColumnResolution{}, false
	}
	return ColumnResolution{Column: column, Kind: schema.MappingKindExplicit, Source: p.Name()}, true
}

// GormTagProvider resolves mappings from `gorm:"column:..."` struct tags.
type GormTagProvider struct{}

func (p *GormTagProvider) Name() string { return "gorm_tag" }

func (p *GormTagProvider) ResolveColumn(field FieldContext, _ MappingMode) (ColumnResolution, bool) {
	raw, ok := field.Tag.Lookup("gorm")
	if !ok {
		return ColumnResolution{}, false
	}
	column, ignored := parseGormColumn(raw)
	if ignored || column == "" {
		return ColumnResolution{}, false
	}
	return ColumnResolution{Column: column, Kind: schema.MappingKindExplicit, Source: p.Name()}, true
}

// InferredNameProvider resolves mappings by inferring snake_case column names
// from field names in non-strict modes.
type InferredNameProvider struct{}

func (p *InferredNameProvider) Name() string { return "field_name" }

func (p *InferredNameProvider) ResolveColumn(field FieldContext, mode MappingMode) (ColumnResolution, bool) {
	if mode == MappingModeStrict {
		return ColumnResolution{}, false
	}
	if field.Name == "" {
		return ColumnResolution{}, false
	}
	if mode == MappingModeBalanced && !field.Exported {
		return ColumnResolution{}, false
	}
	if shouldSkipInference(field.Tag) {
		return ColumnResolution{}, false
	}

	column := toSnakeCase(field.Name)
	if column == "" {
		return ColumnResolution{}, false
	}
	return ColumnResolution{Column: column, Kind: schema.MappingKindInferred, Source: p.Name()}, true
}

// TableNameMethodProvider resolves explicit table names from TableName() string methods.
type TableNameMethodProvider struct{}

func (p *TableNameMethodProvider) Name() string { return "table_name_method" }

func (p *TableNameMethodProvider) ResolveTable(ctx TableContext, _ MappingMode) (TableResolution, bool) {
	tableName := strings.TrimSpace(ctx.MethodTableNames[ctx.TypeName])
	if tableName == "" {
		return TableResolution{}, false
	}
	return TableResolution{
		Table:    tableName,
		Kind:     schema.MappingKindExplicit,
		Explicit: true,
		Source:   p.Name(),
	}, true
}

// InferredTableNameProvider resolves inferred table names from struct type names.
type InferredTableNameProvider struct{}

func (p *InferredTableNameProvider) Name() string { return "type_name" }

func (p *InferredTableNameProvider) ResolveTable(ctx TableContext, _ MappingMode) (TableResolution, bool) {
	if strings.TrimSpace(ctx.TypeName) == "" {
		return TableResolution{}, false
	}
	return TableResolution{
		Table:    strings.ToLower(ctx.TypeName),
		Kind:     schema.MappingKindInferred,
		Explicit: false,
		Source:   p.Name(),
	}, true
}

func primaryTagValue(value string, sep rune) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexRune(value, sep); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func parseGormColumn(raw string) (column string, ignored bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	parts := strings.Split(raw, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if part == "-" || strings.HasPrefix(strings.ToLower(part), "-:") {
			ignored = true
			continue
		}

		kv := strings.SplitN(part, ":", 2)
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		if key != "column" || len(kv) != 2 {
			continue
		}

		candidate := strings.TrimSpace(kv[1])
		if candidate != "" && candidate != "-" {
			column = candidate
		}
	}

	return column, ignored
}

func shouldSkipInference(tag reflect.StructTag) bool {
	if _, ok := tag.Lookup("db"); ok {
		return true
	}
	if raw, ok := tag.Lookup("gorm"); ok {
		_, ignored := parseGormColumn(raw)
		if ignored {
			return true
		}
		return true
	}
	return false
}

func toSnakeCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	var out []rune
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || nextLower {
				out = append(out, '_')
			}
		}
		if r >= 'A' && r <= 'Z' {
			r = r + ('a' - 'A')
		}
		out = append(out, r)
	}
	return string(out)
}

// normalizeGoType converts a Go field type AST expression to a normalized type
// string for schema drift comparison (for example, "string", "int64",
// "time.Time").
func normalizeGoType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		left := normalizeGoType(t.X)
		if left == "" {
			return t.Sel.Name
		}
		return left + "." + t.Sel.Name
	case *ast.StarExpr:
		return normalizeGoType(t.X)
	case *ast.ArrayType:
		return normalizeGoType(t.Elt)
	case *ast.IndexExpr:
		return normalizeGoType(t.X)
	case *ast.IndexListExpr:
		return normalizeGoType(t.X)
	default:
		return ""
	}
}
