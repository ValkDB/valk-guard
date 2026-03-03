package pymodel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/valkdb/valk-guard/internal/schema"
)

func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH, skipping")
	}
}

func writeTempPy(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBasicModel(t *testing.T) {
	requirePython3(t)

	dir := t.TempDir()
	writeTempPy(t, dir, "models.py", `
from sqlalchemy import Column, Integer, String
from sqlalchemy.ext.declarative import declarative_base

Base = declarative_base()

class User(Base):
    __tablename__ = "users"
    id = Column(Integer, primary_key=True)
    email = Column(String(255), nullable=False)
`)

	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	m := models[0]
	if m.Table != "users" {
		t.Errorf("expected table 'users', got %q", m.Table)
	}
	if !m.TableExplicit {
		t.Error("expected table mapping to be explicit")
	}
	if m.Source != schema.ModelSourceSQLAlchemy {
		t.Errorf("expected source %q, got %q", schema.ModelSourceSQLAlchemy, m.Source)
	}
	if len(m.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(m.Columns))
	}

	col0 := m.Columns[0]
	if col0.Name != "id" || col0.Field != "id" || col0.Type != "Integer" {
		t.Errorf("unexpected column[0]: %+v", col0)
	}

	col1 := m.Columns[1]
	if col1.Name != "email" || col1.Field != "email" || col1.Type != "String(255)" {
		t.Errorf("unexpected column[1]: %+v", col1)
	}
}

func TestNoTablename(t *testing.T) {
	requirePython3(t)

	dir := t.TempDir()
	writeTempPy(t, dir, "no_table.py", `
from sqlalchemy import Column, Integer

class NotAModel:
    id = Column(Integer, primary_key=True)
`)

	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 0 {
		t.Fatalf("expected 0 models for class without __tablename__, got %d", len(models))
	}
}

func TestMultipleModels(t *testing.T) {
	requirePython3(t)

	dir := t.TempDir()
	writeTempPy(t, dir, "multi.py", `
from sqlalchemy import Column, Integer, String, Boolean

class User:
    __tablename__ = "users"
    id = Column(Integer, primary_key=True)
    name = Column(String(100))

class Account:
    __tablename__ = "accounts"
    id = Column(Integer, primary_key=True)
    active = Column(Boolean)
`)

	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].Table != "users" {
		t.Errorf("expected first model 'users', got %q", models[0].Table)
	}
	if !models[0].TableExplicit {
		t.Error("expected first model table mapping to be explicit")
	}
	if models[1].Table != "accounts" {
		t.Errorf("expected second model 'accounts', got %q", models[1].Table)
	}
	if !models[1].TableExplicit {
		t.Error("expected second model table mapping to be explicit")
	}
}

func TestExplicitColumnName(t *testing.T) {
	requirePython3(t)

	dir := t.TempDir()
	writeTempPy(t, dir, "explicit.py", `
from sqlalchemy import Column, Integer

class Product:
    __tablename__ = "products"
    product_id = Column("prod_id", Integer, primary_key=True)
`)

	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	col := models[0].Columns[0]
	if !models[0].TableExplicit {
		t.Error("expected explicit table mapping")
	}
	if col.Name != "prod_id" {
		t.Errorf("expected column name 'prod_id', got %q", col.Name)
	}
	if col.Field != "product_id" {
		t.Errorf("expected field name 'product_id', got %q", col.Field)
	}
	if col.Type != "Integer" {
		t.Errorf("expected type 'Integer', got %q", col.Type)
	}
}

func TestQuickRejectSkipsNonSQLAlchemy(t *testing.T) {
	dir := t.TempDir()
	writeTempPy(t, dir, "plain.py", `
class Foo:
    x = 42
    y = "hello"
`)

	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 0 {
		t.Fatalf("expected 0 models for non-SQLAlchemy file, got %d", len(models))
	}
}

func TestEmptyPaths(t *testing.T) {
	ext := &Extractor{}
	models, err := ext.ExtractModels(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if models != nil {
		t.Fatalf("expected nil models for empty paths, got %v", models)
	}
}
