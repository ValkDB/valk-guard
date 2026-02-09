package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func FuzzRawSQLScanner(f *testing.F) {
	seedInputs := []string{
		"SELECT 1;",
		"SELECT 'a; b';",
		"/* comment ; */ SELECT 1;",
		"CREATE FUNCTION f() RETURNS void AS $$ BEGIN RAISE NOTICE 'x;y'; END; $$ LANGUAGE plpgsql;",
		"-- valk-guard:disable VG001\nSELECT * FROM users;",
	}
	for _, seed := range seedInputs {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "fuzz.sql")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write fuzz input: %v", err)
		}

		s := &RawSQLScanner{}
		stmts, err := Collect(s.Scan(context.Background(), []string{path}))
		if err != nil {
			t.Fatalf("scan error: %v", err)
		}

		for _, stmt := range stmts {
			_, _ = ParseStatement(stmt.SQL)
		}
	})
}
