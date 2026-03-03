// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package gomodel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func writeGoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantDefs []schema.ModelDef
	}{
		{
			name: "basic struct with db tags",
			src: `package models

type User struct {
	ID   int    ` + "`db:\"id\"`" + `
	Name string ` + "`db:\"name\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "user",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 4},
						{Name: "name", Field: "Name", Line: 5},
					},
					Line: 3,
				},
			},
		},
		{
			name: "struct with no db tags",
			src: `package models

type Config struct {
	Host string
	Port int
}
`,
			wantDefs: nil,
		},
		{
			name: "struct with db dash fields skipped",
			src: `package models

type Account struct {
	ID      int    ` + "`db:\"id\"`" + `
	Secret  string ` + "`db:\"-\"`" + `
	Email   string ` + "`db:\"email\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "account",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 4},
						{Name: "email", Field: "Email", Line: 6},
					},
					Line: 3,
				},
			},
		},
		{
			name: "mixed fields with and without db tags",
			src: `package models

type Order struct {
	ID     int    ` + "`db:\"id\"`" + `
	cached bool
	Total  float64 ` + "`db:\"total\"`" + `
	note   string  ` + "`json:\"note\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "order",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 4},
						{Name: "total", Field: "Total", Line: 6},
					},
					Line: 3,
				},
			},
		},
		{
			name: "column types are captured",
			src: `package models

import "time"

type Event struct {
	ID        int64     ` + "`db:\"id\"`" + `
	Name      string    ` + "`db:\"name\"`" + `
	CreatedAt time.Time ` + "`db:\"created_at\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "event",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Type: "int64", Line: 6},
						{Name: "name", Field: "Name", Type: "string", Line: 7},
						{Name: "created_at", Field: "CreatedAt", Type: "time.Time", Line: 8},
					},
					Line: 5,
				},
			},
		},
		{
			name: "tag with comma options",
			src: `package models

type Profile struct {
	ID   int    ` + "`db:\"id\"`" + `
	Name string ` + "`db:\"name,omitempty\"`" + `
	Bio  string ` + "`db:\"bio,readonly,pk\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "profile",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 4},
						{Name: "name", Field: "Name", Line: 5},
						{Name: "bio", Field: "Bio", Line: 6},
					},
					Line: 3,
				},
			},
		},
		{
			name: "multiple structs in one file",
			src: `package models

type User struct {
	ID   int    ` + "`db:\"id\"`" + `
	Name string ` + "`db:\"name\"`" + `
}

type Product struct {
	SKU   string  ` + "`db:\"sku\"`" + `
	Price float64 ` + "`db:\"price\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "user",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 4},
						{Name: "name", Field: "Name", Line: 5},
					},
					Line: 3,
				},
				{
					Table: "product",
					Columns: []schema.ModelColumn{
						{Name: "sku", Field: "SKU", Line: 9},
						{Name: "price", Field: "Price", Line: 10},
					},
					Line: 8,
				},
			},
		},
		{
			name: "embedded struct without db tags skipped",
			src: `package models

type Base struct {
	CreatedAt string
}

type Item struct {
	Base
	ID   int    ` + "`db:\"id\"`" + `
	Name string ` + "`db:\"name\"`" + `
}
`,
			wantDefs: []schema.ModelDef{
				{
					Table: "item",
					Columns: []schema.ModelColumn{
						{Name: "id", Field: "ID", Line: 9},
						{Name: "name", Field: "Name", Line: 10},
					},
					Line: 7,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeGoFile(t, dir, "models.go", tt.src)

			ext := &Extractor{}
			got, err := ext.ExtractModels(context.Background(), []string{dir})
			if err != nil {
				t.Fatalf("ExtractModels() error: %v", err)
			}

			if len(got) != len(tt.wantDefs) {
				t.Fatalf("got %d models, want %d", len(got), len(tt.wantDefs))
			}

			for i, want := range tt.wantDefs {
				g := got[i]
				if g.Table != want.Table {
					t.Errorf("model[%d].Table = %q, want %q", i, g.Table, want.Table)
				}
				if g.Source != schema.ModelSourceGo {
					t.Errorf("model[%d].Source = %q, want %q", i, g.Source, schema.ModelSourceGo)
				}
				if g.TableExplicit {
					t.Errorf("model[%d].TableExplicit = true, want false", i)
				}
				if g.Line != want.Line {
					t.Errorf("model[%d].Line = %d, want %d", i, g.Line, want.Line)
				}
				if len(g.Columns) != len(want.Columns) {
					t.Fatalf("model[%d] got %d columns, want %d", i, len(g.Columns), len(want.Columns))
				}
				for j, wc := range want.Columns {
					gc := g.Columns[j]
					if gc.Name != wc.Name {
						t.Errorf("model[%d].col[%d].Name = %q, want %q", i, j, gc.Name, wc.Name)
					}
					if gc.Field != wc.Field {
						t.Errorf("model[%d].col[%d].Field = %q, want %q", i, j, gc.Field, wc.Field)
					}
					if wc.Type != "" && gc.Type != wc.Type {
						t.Errorf("model[%d].col[%d].Type = %q, want %q", i, j, gc.Type, wc.Type)
					}
					if gc.Line != wc.Line {
						t.Errorf("model[%d].col[%d].Line = %d, want %d", i, j, gc.Line, wc.Line)
					}
				}
			}
		})
	}
}

func TestExtractModels_EmptyPaths(t *testing.T) {
	t.Parallel()

	ext := &Extractor{}
	got, err := ext.ExtractModels(context.Background(), []string{})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d models, want 0", len(got))
	}
}

func TestExtractModels_GormTagsAndTableNameMethod(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "models.go", `package models

type User struct {
	ID    int    `+"`gorm:\"column:id\"`"+`
	Email string `+"`gorm:\"column:email;type:text\"`"+`
}

func (User) TableName() string {
	return "users"
}
`)

	ext := &Extractor{Mode: MappingModeStrict}
	got, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d models, want 1", len(got))
	}

	m := got[0]
	if m.Table != "users" {
		t.Fatalf("Table = %q, want users", m.Table)
	}
	if !m.TableExplicit {
		t.Fatalf("TableExplicit = false, want true")
	}
	if m.TableMappingKind != schema.MappingKindExplicit {
		t.Fatalf("TableMappingKind = %q, want %q", m.TableMappingKind, schema.MappingKindExplicit)
	}
	if m.TableMappingSource != "table_name_method" {
		t.Fatalf("TableMappingSource = %q, want table_name_method", m.TableMappingSource)
	}
	if len(m.Columns) != 2 {
		t.Fatalf("got %d columns, want 2", len(m.Columns))
	}
	for _, col := range m.Columns {
		if col.MappingKind != schema.MappingKindExplicit {
			t.Fatalf("column %q MappingKind = %q, want %q", col.Name, col.MappingKind, schema.MappingKindExplicit)
		}
		if col.MappingSource != "gorm_tag" {
			t.Fatalf("column %q MappingSource = %q, want gorm_tag", col.Name, col.MappingSource)
		}
	}
}

func TestExtractModels_BalancedInferenceUsesExportedFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "models.go", `package models

type Session struct {
	ID        int
	CreatedAt string
	token     string
}
`)

	ext := &Extractor{Mode: MappingModeBalanced}
	got, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d models, want 1", len(got))
	}
	if len(got[0].Columns) != 2 {
		t.Fatalf("got %d columns, want 2", len(got[0].Columns))
	}

	if got[0].Columns[0].Name != "id" || got[0].Columns[1].Name != "created_at" {
		t.Fatalf("unexpected inferred columns: %+v", got[0].Columns)
	}
	for _, col := range got[0].Columns {
		if col.MappingKind != schema.MappingKindInferred || col.MappingSource != "field_name" {
			t.Fatalf("unexpected mapping metadata for %q: kind=%q source=%q", col.Name, col.MappingKind, col.MappingSource)
		}
	}
}

func TestExtractModels_PermissiveInferenceIncludesUnexportedFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "models.go", `package models

type Session struct {
	ID    int
	token string
}
`)

	ext := &Extractor{Mode: MappingModePermissive}
	got, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d models, want 1", len(got))
	}

	var names []string
	for _, col := range got[0].Columns {
		names = append(names, col.Name)
	}
	if strings.Join(names, ",") != "id,token" {
		t.Fatalf("unexpected permissive columns: %+v", got[0].Columns)
	}
}

func TestExtractModels_MultiNameFieldExpandsAllNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "models.go", `package models

type Pair struct {
	A, B string `+"`db:\"name\"`"+`
}
`)

	ext := &Extractor{}
	got, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d models, want 1", len(got))
	}
	if len(got[0].Columns) != 2 {
		t.Fatalf("got %d columns, want 2", len(got[0].Columns))
	}
	if got[0].Columns[0].Field != "A" || got[0].Columns[1].Field != "B" {
		t.Fatalf("unexpected multi-name expansion: %+v", got[0].Columns)
	}
}

type dummyColumnProvider struct{}

func (p *dummyColumnProvider) Name() string { return "dummy_column" }

func (p *dummyColumnProvider) ResolveColumn(field FieldContext, _ MappingMode) (ColumnResolution, bool) {
	if field.Name == "Special" {
		return ColumnResolution{
			Column: "special_column",
			Kind:   schema.MappingKindExplicit,
			Source: p.Name(),
		}, true
	}
	return ColumnResolution{}, false
}

type dummyTableProvider struct{}

func (p *dummyTableProvider) Name() string { return "dummy_table" }

func (p *dummyTableProvider) ResolveTable(ctx TableContext, _ MappingMode) (TableResolution, bool) {
	if ctx.TypeName == "Custom" {
		return TableResolution{
			Table:    "custom_table",
			Kind:     schema.MappingKindExplicit,
			Explicit: true,
			Source:   p.Name(),
		}, true
	}
	return TableResolution{}, false
}

func TestExtractor_CustomProviderPipeline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFile(t, dir, "models.go", `package models

type Custom struct {
	Special string
}
`)

	ext := &Extractor{
		Mode:            MappingModeStrict,
		ColumnProviders: []ColumnMappingProvider{&dummyColumnProvider{}},
		TableProviders:  []TableMappingProvider{&dummyTableProvider{}},
	}

	got, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d models, want 1", len(got))
	}

	m := got[0]
	if m.Table != "custom_table" || !m.TableExplicit {
		t.Fatalf("unexpected table mapping: table=%q explicit=%v", m.Table, m.TableExplicit)
	}
	if len(m.Columns) != 1 {
		t.Fatalf("got %d columns, want 1", len(m.Columns))
	}
	if m.Columns[0].Name != "special_column" {
		t.Fatalf("unexpected column mapping: %+v", m.Columns[0])
	}
}
