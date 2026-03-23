// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package csharp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
)

// helper writes a .cs file in a temp dir, scans it, and returns statements.
func scanCS(t *testing.T, src string) []scanner.SQLStatement {
	t.Helper()
	tmpDir := t.TempDir()
	csFile := filepath.Join(tmpDir, "test.cs")
	if err := os.WriteFile(csFile, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	return stmts
}

func TestDirectLiteral(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore;

class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := "DELETE FROM temp_data WHERE created_at < NOW()"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
	if stmts[0].Engine != scanner.EngineCSharp {
		t.Errorf("engine = %q, want %q", stmts[0].Engine, scanner.EngineCSharp)
	}
}

func TestVerbatimString(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw(@"UPDATE users
            SET active = false
            WHERE last_login < '2024-01-01'");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0].SQL, "UPDATE users") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
	if !strings.Contains(stmts[0].SQL, "WHERE last_login") {
		t.Errorf("missing WHERE in SQL: %q", stmts[0].SQL)
	}
}

func TestRawStringLiteral(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw("""
            SELECT id, name
            FROM users
            WHERE active = true
            LIMIT 100
            """);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0].SQL, "SELECT id, name") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
	if !strings.Contains(stmts[0].SQL, "LIMIT 100") {
		t.Errorf("missing LIMIT in SQL: %q", stmts[0].SQL)
	}
}

func TestInterpolatedString(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int userId) {
        db.Database.ExecuteSqlInterpolated($"DELETE FROM users WHERE id = {userId}");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "DELETE FROM users WHERE id = $1"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestLocalVariablePassedToExecuteSqlRaw(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var sql = "SELECT id FROM users WHERE active = true LIMIT 50";
        db.Database.ExecuteSqlRaw(sql);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "SELECT id FROM users WHERE active = true LIMIT 50"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestDatabaseFacadeHelper(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore.Infrastructure;

class SqlRunner {
    public void CleanUp(DatabaseFacade database) {
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW() - INTERVAL '1 day'");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0].SQL, "DELETE FROM temp_data") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
}

func TestLocalDatabaseFacadeReceiver(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore.Infrastructure;

class Repo {
    void Run(MyDbContext db) {
        DatabaseFacade database = db.Database;
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0].SQL, "DELETE FROM temp_data") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
}

func TestRejectsNonDatabaseReceiverWithMatchingMethodName(t *testing.T) {
	src := `class FakeRunner {
    public void ExecuteSqlRaw(string sql) {}
}

class Repo {
    void Run(FakeRunner runner) {
        runner.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for non-EF receiver, got %d: %+v", len(stmts), stmts)
	}
}

func TestAsyncVariants(t *testing.T) {
	src := `class Repo {
    async Task Run(MyDbContext db, int id) {
        await db.Database.ExecuteSqlRawAsync("SELECT 1");
        await db.Database.ExecuteSqlInterpolatedAsync($"DELETE FROM users WHERE id = {id}");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 2 {
		t.Fatalf("expected 2, got %d", len(stmts))
	}
	if stmts[0].SQL != "SELECT 1" {
		t.Errorf("stmt[0] SQL = %q, want %q", stmts[0].SQL, "SELECT 1")
	}
	want := "DELETE FROM users WHERE id = $1"
	if stmts[1].SQL != want {
		t.Errorf("stmt[1] SQL = %q, want %q", stmts[1].SQL, want)
	}
}

func TestConcatenation(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw("SELECT * FROM " + "users WHERE id = 1 LIMIT 1");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "SELECT * FROM users WHERE id = 1 LIMIT 1"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestVariableResolutionStaysWithinMethodScope(t *testing.T) {
	src := `class Repo {
    void Prepare() {
        var sql = "DELETE FROM temp_data WHERE created_at < NOW()";
    }

    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw(sql);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for cross-method variable reference, got %d: %+v", len(stmts), stmts)
	}
}

func TestUnresolvedDynamicSkipped(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, string table) {
        db.Database.ExecuteSqlRaw("SELECT * FROM " + table);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements for dynamic SQL, got %d: %+v", len(stmts), stmts)
	}
}

func TestFormatStringPlaceholders(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int id, string name) {
        db.Database.ExecuteSqlRaw("SELECT * FROM users WHERE id = {0} AND name = {1} LIMIT 1", id, name);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "SELECT * FROM users WHERE id = $1 AND name = $2 LIMIT 1"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestDisableDirective(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        // valk-guard:disable VG001
        db.Database.ExecuteSqlRaw("SELECT * FROM users LIMIT 10");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if len(stmts[0].Disabled) != 1 || stmts[0].Disabled[0] != "VG001" {
		t.Errorf("expected disabled=[VG001], got %v", stmts[0].Disabled)
	}
}

func TestSkipsCallsInsideComments(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        // db.Database.ExecuteSqlRaw("SELECT 1");
        /* db.Database.ExecuteSqlRaw("SELECT 2"); */
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0, got %d: %+v", len(stmts), stmts)
	}
}

func TestSkipsCallsInsideStrings(t *testing.T) {
	src := `class Repo {
    void Run() {
        var s = "db.Database.ExecuteSqlRaw(\"SELECT 1\")";
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0, got %d: %+v", len(stmts), stmts)
	}
}

func TestVerbatimInterpolated(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int id) {
        db.Database.ExecuteSqlInterpolated($@"UPDATE users
            SET active = false
            WHERE id = {id}");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0].SQL, "UPDATE users") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
	if !strings.Contains(stmts[0].SQL, "$1") {
		t.Errorf("missing placeholder in SQL: %q", stmts[0].SQL)
	}
}

func TestMultipleInterpolationPlaceholders(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int id, string name, bool active) {
        db.Database.ExecuteSqlInterpolated(
            $"UPDATE users SET name = {name}, active = {active} WHERE id = {id}");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "UPDATE users SET name = $1, active = $2 WHERE id = $3"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Scanner{}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(stmts) != 0 {
		t.Errorf("expected 0, got %d", len(stmts))
	}
}

func TestNonSQLStringSkipped(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw("hello world");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for non-SQL string, got %d", len(stmts))
	}
}

func TestVerbatimStringVariableAssignment(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        string query = @"DELETE FROM temp_data
            WHERE created_at < NOW()";
        db.Database.ExecuteSqlRaw(query);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0].SQL, "DELETE FROM temp_data") {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}
}

func TestPropertyAssignmentNotCaptured(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        obj.Query = "SELECT * FROM secrets";
        db.Database.ExecuteSqlRaw(Query);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 (property assignment should not be captured as var), got %d: %+v", len(stmts), stmts)
	}
}

func TestConcatenationWithVariable(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var prefix = "SELECT id, name FROM ";
        db.Database.ExecuteSqlRaw(prefix + "users WHERE active = true LIMIT 10");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "SELECT id, name FROM users WHERE active = true LIMIT 10"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}
