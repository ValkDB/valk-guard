package gomodel

import (
	"context"
	"go/ast"
	"go/token"
	"reflect"
	"strings"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// Extractor extracts model definitions from Go source files by inspecting
// struct tags. Structs with at least one `db:"column_name"` tag are treated
// as ORM models.
type Extractor struct{}

// ExtractModels walks the given paths for Go source files and returns a
// ModelDef for every struct that has at least one `db` struct tag.
func (e *Extractor) ExtractModels(ctx context.Context, paths []string) ([]schema.ModelDef, error) {
	var models []schema.ModelDef

	err := scanner.WalkGoFiles(ctx, paths, func(path string, fset *token.FileSet, file *ast.File, _ []byte) error {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}

			var cols []schema.ModelColumn
			for _, field := range st.Fields.List {
				if field.Tag == nil {
					continue
				}
				// Tag value includes the surrounding backticks; strip them.
				raw := field.Tag.Value
				if len(raw) < 2 {
					continue
				}
				tag := reflect.StructTag(raw[1 : len(raw)-1])

				dbVal, ok := tag.Lookup("db")
				if !ok || dbVal == "-" {
					continue
				}

				// Take only the part before the first comma.
				if idx := strings.IndexByte(dbVal, ','); idx != -1 {
					dbVal = dbVal[:idx]
				}
				if dbVal == "" {
					continue
				}

				fieldName := ""
				if len(field.Names) > 0 {
					fieldName = field.Names[0].Name
				}

				cols = append(cols, schema.ModelColumn{
					Name:  dbVal,
					Field: fieldName,
					Line:  fset.Position(field.Pos()).Line,
				})
			}

			if len(cols) == 0 {
				return true
			}

			models = append(models, schema.ModelDef{
				Table:   strings.ToLower(ts.Name.Name),
				Columns: cols,
				File:    path,
				Line:    fset.Position(ts.Pos()).Line,
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
