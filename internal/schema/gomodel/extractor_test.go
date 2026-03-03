package gomodel

import (
	"context"
	"os"
	"path/filepath"
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
					if gc.Line != wc.Line {
						t.Errorf("model[%d].col[%d].Line = %d, want %d", i, j, gc.Line, wc.Line)
					}
				}
			}
		})
	}
}

func TestExtractModels_EmptyPaths(t *testing.T) {
	ext := &Extractor{}
	got, err := ext.ExtractModels(context.Background(), []string{})
	if err != nil {
		t.Fatalf("ExtractModels() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d models, want 0", len(got))
	}
}
