// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package csharp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scannertest"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("VALK_FAKE_DOTNET"); mode != "" {
		switch mode {
		case "sleep":
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		case "malformed-json":
			_, _ = os.Stdout.WriteString("{")
			os.Exit(0)
		default:
			_, _ = os.Stdout.WriteString("[]")
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

// requireDotnet skips Roslyn-dependent scanner tests when the .NET SDK is not
// available in the local developer environment. GitHub Actions installs .NET,
// so these tests still run in CI.
func requireDotnet(t *testing.T) {
	t.Helper()
	if path := os.Getenv("VALK_DOTNET_PATH"); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("VALK_DOTNET_PATH is set but not usable: %v", err)
		}
		return
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		if os.Getenv("VALK_REQUIRE_DOTNET") == "1" {
			t.Fatalf("dotnet SDK is required for C# Roslyn scanner tests: %v", err)
		}
		t.Skip("dotnet SDK is required for C# Roslyn scanner tests")
	}
}

// dotnetPathForTest resolves the dotnet executable for local and CI test runs.
func dotnetPathForTest() string {
	if path := os.Getenv("VALK_DOTNET_PATH"); path != "" {
		return path
	}
	return ""
}

// helper writes a .cs file in a temp dir, scans it, and returns statements.
func scanCS(t *testing.T, src string) []scanner.SQLStatement {
	t.Helper()
	requireDotnet(t)
	return scanCSWithScanner(t, &Scanner{DotnetPath: dotnetPathForTest(), ProjectPath: testRoslynProjectPath()}, src)
}

// scanCSWithScanner writes a .cs file and scans it with the provided scanner.
func scanCSWithScanner(t *testing.T, s *Scanner, src string) []scanner.SQLStatement {
	t.Helper()
	tmpDir := t.TempDir()
	if os.Getenv("VALK_DOTNET_PATH") != "" {
		var err error
		tmpDir, err = os.MkdirTemp(".", ".csharp-test-*")
		if err != nil {
			t.Fatalf("create local temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	}
	csFile := filepath.Join(tmpDir, "test.cs")
	if err := os.WriteFile(csFile, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	return stmts
}

// testRoslynProjectPath returns the checked-in extractor project so CI can use
// incremental dotnet builds across scanner tests.
func testRoslynProjectPath() string {
	return filepath.Join("roslynextractor", "RoslynExtractor.csproj")
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

func TestLocalConcatenatedVariablePreservesSql(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var where = " WHERE active = true";
        var sql = "SELECT id FROM users" + where + " LIMIT 50";
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

func TestLocalInterpolatedVariablePreservesPlaceholder(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int id) {
        var sql = $"DELETE FROM users WHERE id = {id}";
        db.Database.ExecuteSqlInterpolated(sql);
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

func TestLocalVariableReassignmentUsesLatestBeforeCall(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var sql = "SELECT * FROM users";
        sql = "DELETE FROM users WHERE active = false";
        db.Database.ExecuteSqlRaw(sql);
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d", len(stmts))
	}
	want := "DELETE FROM users WHERE active = false"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestLocalInterpolationArgumentCanBeAnotherLocal(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var id = 42;
        var sql = $"DELETE FROM users WHERE id = {id}";
        db.Database.ExecuteSqlInterpolated(sql);
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

func TestDatabaseFacadeReceiverReassignmentBeforeCall(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore.Infrastructure;

class Repo {
    void Run(MyDbContext db, FakeRunner fake) {
        DatabaseFacade database = db.Database;
        database = fake;
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 after non-facade reassignment, got %d: %+v", len(stmts), stmts)
	}
}

func TestDatabaseFacadeReceiverConditionalReassignmentIsConservative(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore.Infrastructure;

class Repo {
    void Run(MyDbContext db, FakeRunner fake, bool flag) {
        DatabaseFacade database = db.Database;
        if (flag) {
            database = fake;
        }
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for conditional receiver reassignment, got %d: %+v", len(stmts), stmts)
	}
}

func TestDatabaseFacadeReceiverNestedScopeDoesNotLeak(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore.Infrastructure;

class Repo {
    void Run(MyDbContext db) {
        {
            DatabaseFacade database = db.Database;
        }
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for nested-scope facade local, got %d: %+v", len(stmts), stmts)
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

func TestNoCSharpFilesDoesNotRequireDotnet(t *testing.T) {
	tmpDir := t.TempDir()
	textFile := filepath.Join(tmpDir, "plain.txt")
	if err := os.WriteFile(textFile, []byte(`class Plain { string Name => "not sql"; }`), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	s := &Scanner{DotnetPath: "missing-dotnet-for-test"}
	stmts, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d: %+v", len(stmts), stmts)
	}
}

func TestCollectCandidatesReturnsAllCSharpFiles(t *testing.T) {
	tmpDir := t.TempDir()
	csFile := filepath.Join(tmpDir, "plain.cs")
	if err := os.WriteFile(csFile, []byte(`class Plain { string Name => "not sql"; }`), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	candidates, err := collectCandidates(context.Background(), []string{tmpDir})
	if err != nil {
		t.Fatalf("collectCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0] != csFile {
		t.Fatalf("expected C# candidate %q, got %+v", csFile, candidates)
	}
}

func TestCachedRoslynProjectMaterializesEmbeddedFiles(t *testing.T) {
	cacheRoot := t.TempDir()
	oldUserCacheDir := userCacheDir
	userCacheDir = func() (string, error) { return cacheRoot, nil }
	t.Cleanup(func() { userCacheDir = oldUserCacheDir })

	project, err := (&Scanner{}).roslynProject()
	if err != nil {
		t.Fatalf("roslynProject() error = %v", err)
	}
	if !project.cached {
		t.Fatal("expected embedded Roslyn project to use cache")
	}
	if !strings.HasPrefix(project.projectPath, cacheRoot) {
		t.Fatalf("expected project path under cache root %q, got %q", cacheRoot, project.projectPath)
	}
	if _, err := os.Stat(project.projectPath); err != nil {
		t.Fatalf("expected cached project file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(project.projectPath), "Program.cs")); err != nil {
		t.Fatalf("expected cached program file: %v", err)
	}

	second, err := (&Scanner{}).roslynProject()
	if err != nil {
		t.Fatalf("second roslynProject() error = %v", err)
	}
	if second.projectPath != project.projectPath {
		t.Fatalf("expected stable cache path, got %q then %q", project.projectPath, second.projectPath)
	}
	if !strings.HasSuffix(project.executablePath, filepath.Join("publish", cachedExtractorName())) {
		t.Fatalf("expected cached executable under publish dir, got %q", project.executablePath)
	}
}

func TestPathForDotnetConvertsWSLPathsForWindowsDotnet(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("WSL path conversion only applies on linux")
	}
	if got := pathForDotnet("/mnt/c/Program Files/dotnet/dotnet.exe", "/mnt/c/users/eitam/cache/project.csproj"); got != `C:\users\eitam\cache\project.csproj` {
		t.Fatalf("unexpected converted path: %q", got)
	}
}

func TestCSharpFilesRequireDotnet(t *testing.T) {
	src := `using Microsoft.EntityFrameworkCore;

class Repo {
    void Run(MyDbContext db) {
        db.Database.ExecuteSqlRaw("SELECT 1");
    }
}`
	tmpDir := t.TempDir()
	if os.Getenv("VALK_DOTNET_PATH") != "" {
		var err error
		tmpDir, err = os.MkdirTemp(".", ".csharp-test-*")
		if err != nil {
			t.Fatalf("create local temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	}
	csFile := filepath.Join(tmpDir, "test.cs")
	if err := os.WriteFile(csFile, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	s := &Scanner{DotnetPath: "missing-dotnet-for-test"}
	_, err := scanner.Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err == nil {
		t.Fatal("expected missing dotnet error")
	}
	if !strings.Contains(err.Error(), ".NET SDK is required") {
		t.Fatalf("expected .NET SDK error, got %v", err)
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

func TestFromSqlRawExtractedFromDbSet(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        var users = db.Users.FromSqlRaw("SELECT * FROM users WHERE active = true").ToList();
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d: %+v", len(stmts), stmts)
	}
	want := "SELECT * FROM users WHERE active = true"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestSqlQueryInterpolatedExtractedFromDatabase(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int id) {
        var ids = db.Database.SqlQueryInterpolated<int>($"SELECT id FROM users WHERE id = {id}").ToList();
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 1 {
		t.Fatalf("expected 1, got %d: %+v", len(stmts), stmts)
	}
	want := "SELECT id FROM users WHERE id = $1"
	if stmts[0].SQL != want {
		t.Errorf("SQL = %q, want %q", stmts[0].SQL, want)
	}
}

func TestRawEFSQLSelectForUpdateTriggersRule(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.FromSqlRaw("SELECT id FROM users FOR UPDATE").ToList();
    }
}`
	stmts := scanCS(t, src)
	findingsByRule := scannertest.CollectFindingsByRule(t, stmts)
	if findingsByRule["VG006"] == 0 {
		t.Fatalf("expected VG006 from raw EF SQL FOR UPDATE without WHERE, got %+v (stmts: %+v)", findingsByRule, stmts)
	}
}

func TestLINQChainsGenerateSyntheticSQLAndTriggerRules(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Include(u => u.Orders).ToList();
        db.Users.Where(u => u.Email.Contains("@gmail.com")).ToList();
        db.Logs.Include(l => l.User).Select(l => new { l.Id, l.Message }).ToList();
        db.Users.ExecuteDelete();
        db.Users.ExecuteUpdate(s => s.SetProperty(u => u.Active, false));
    }
}`
	stmts := scanCS(t, src)
	findingsByRule := scannertest.CollectFindingsByRule(t, stmts)
	for _, ruleID := range []string{"VG001", "VG002", "VG003", "VG004", "VG005"} {
		if findingsByRule[ruleID] == 0 {
			t.Fatalf("expected %s from C# synthetic LINQ chains, got none (findings: %+v, stmts: %+v)", ruleID, findingsByRule, stmts)
		}
	}
	if !scannertest.HasSQLContaining(stmts, "/* valk-guard:synthetic csharp-efcore */ SELECT * FROM Users LEFT JOIN Orders ON 1=1") {
		t.Fatalf("expected synthetic SELECT with include join, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "Email LIKE '%@gmail.com%'") {
		t.Fatalf("expected synthetic LIKE predicate, got %+v", stmts)
	}
}

func TestLINQChainsFromStandaloneWhereGenerateSyntheticSQL(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Where(u => u.Email.Contains("@gmail.com")).ToList();
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "Email LIKE '%@gmail.com%'") {
		t.Fatalf("expected standalone Where synthetic LIKE predicate, got %+v", stmts)
	}
}

func TestLINQCountAndAnyAvoidSelectStarAndUnboundedFindings(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Count();
        db.Users.Any();
    }
}`
	stmts := scanCS(t, src)
	findingsByRule := scannertest.CollectFindingsByRule(t, stmts)
	if findingsByRule["VG001"] != 0 || findingsByRule["VG004"] != 0 {
		t.Fatalf("expected Count/Any chains to avoid VG001/VG004, got %+v (stmts: %+v)", findingsByRule, stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "SELECT COUNT(*) FROM Users") {
		t.Fatalf("expected Count synthetic aggregate SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "SELECT 1 FROM Users LIMIT 1") {
		t.Fatalf("expected Any synthetic bounded SQL, got %+v", stmts)
	}
}

func TestDbSetQualifiedGenericTypeUsesRightmostTableName(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Set<My.App.User>().Count();
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "SELECT COUNT(*) FROM User") {
		t.Fatalf("expected qualified generic DbSet table to use rightmost type name, got %+v", stmts)
	}
}

func TestLINQILikeGeneratesILikePredicate(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Where(u => EF.Functions.ILike(u.Email, "%@gmail.com")).ToList();
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "Email ILIKE '%@gmail.com'") {
		t.Fatalf("expected ILike synthetic predicate, got %+v", stmts)
	}
}

func TestUserDefinedILikeIsNotRenderedAsILIKE(t *testing.T) {
	src := `class Helpers {
    public static bool ILike(string a, string b) { return false; }
}
class Repo {
    void Run(MyDbContext db) {
        db.Users.Where(u => Helpers.ILike(u.Email, "%@gmail.com")).ToList();
    }
}`
	stmts := scanCS(t, src)
	if scannertest.HasSQLContaining(stmts, "ILIKE") {
		t.Fatalf("expected user-defined ILike helper to not produce ILIKE, got %+v", stmts)
	}
	if scannertest.HasSQLContaining(stmts, "LIKE '%@gmail.com'") {
		t.Fatalf("expected user-defined ILike helper to not produce LIKE either, got %+v", stmts)
	}
}

func TestWhereThenAnyPredicatesUseUniquePlaceholders(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int userId, string name) {
        db.Users.Where(u => u.Id == userId).Any(u => u.Name == name);
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "Id = $1") {
		t.Fatalf("expected first predicate to bind $1, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "Name = $2") {
		t.Fatalf("expected terminal Any predicate to advance to $2, got %+v", stmts)
	}
	if scannertest.HasSQLContaining(stmts, "Name = $1") {
		t.Fatalf("expected terminal Any predicate to not collide on $1, got %+v", stmts)
	}
}

func TestWhereThenAllPredicatesUseUniquePlaceholders(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int userId, bool active) {
        db.Users.Where(u => u.Id == userId).All(u => u.Active == active);
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "Id = $1") {
		t.Fatalf("expected first predicate to bind $1, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "Active = $2") {
		t.Fatalf("expected terminal All predicate to advance to $2, got %+v", stmts)
	}
}

func TestLINQDeleteAndUpdateIgnoreIncludeTargets(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Include(u => u.Roles).ExecuteDelete();
        db.Users.Include(u => u.Profile).ExecuteUpdate(s => s.SetProperty(u => u.Active, false));
    }
}`
	stmts := scanCS(t, src)
	if scannertest.HasSQLContaining(stmts, "USING Roles") || scannertest.HasSQLContaining(stmts, "FROM Profile") {
		t.Fatalf("expected Include targets to be ignored for write SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "DELETE FROM Users") {
		t.Fatalf("expected delete synthetic SQL, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "UPDATE Users SET Active = $1") {
		t.Fatalf("expected update synthetic SQL with placeholder set clause, got %+v", stmts)
	}
}

func TestLINQDeleteAndUpdatePreserveExplicitJoinTargets(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Join(db.Roles, u => u.Id, r => r.UserId, (u, r) => u).ExecuteDelete();
        db.Users.Join(db.Profiles, u => u.Id, p => p.UserId, (u, p) => u).ExecuteUpdate(s => s.SetProperty(u => u.Active, false));
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "DELETE FROM Users USING Roles") {
		t.Fatalf("expected delete synthetic SQL to preserve explicit join target, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "UPDATE Users SET Active = $1 FROM Profiles") {
		t.Fatalf("expected update synthetic SQL to preserve explicit join target, got %+v", stmts)
	}
}

func TestLINQWhereAndTakeAvoidMissingWhereAndLimitFindings(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db) {
        db.Users.Where(u => u.Active == true).Take(25).ToList();
        db.Users.Where(u => u.Id == 7).ExecuteDelete();
        db.Users.Where(u => u.Id == 8).ExecuteUpdate(s => s.SetProperty(u => u.Active, false));
    }
}`
	stmts := scanCS(t, src)
	findingsByRule := scannertest.CollectFindingsByRule(t, stmts)
	if findingsByRule["VG002"] != 0 || findingsByRule["VG003"] != 0 || findingsByRule["VG004"] != 0 {
		t.Fatalf("expected filtered/taken chains to avoid VG002/VG003/VG004, got %+v (stmts: %+v)", findingsByRule, stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "WHERE Active = TRUE LIMIT 25") {
		t.Fatalf("expected synthesized WHERE and LIMIT, got %+v", stmts)
	}
}

func TestRejectsDatabaseNamedNonFacadeReceiver(t *testing.T) {
	src := `class FakeRunner {
    public void ExecuteSqlRaw(string sql) {}
}

class Repo {
    void Run(FakeRunner database) {
        database.ExecuteSqlRaw("DELETE FROM temp_data WHERE created_at < NOW()");
    }
}`
	stmts := scanCS(t, src)
	if len(stmts) != 0 {
		t.Fatalf("expected 0 for non-DatabaseFacade parameter named database, got %d: %+v", len(stmts), stmts)
	}
}

func TestLINQParityFeatureClauses(t *testing.T) {
	src := `class Repo {
    void Run(MyDbContext db, int[] ids, int minId) {
        db.Users
            .Distinct()
            .Where(u => ids.Contains(u.Id))
            .OrderBy(u => u.Email)
            .ThenByDescending(u => u.Id)
            .GroupBy(u => u.Email)
            .Where(u => u.Id > minId)
            .Skip(20)
            .Take(10)
            .ToList();
        db.Users.Where(u => !ids.Contains(u.Id)).Take(1).ToList();
        db.Orders.Sum(o => o.Total);
    }
}`
	stmts := scanCS(t, src)
	for _, want := range []string{
		"SELECT DISTINCT * FROM Users",
		"WHERE Id IN ($1, $2, $3)",
		"GROUP BY Email HAVING Id > $4",
		"ORDER BY Email ASC, Id DESC LIMIT 10 OFFSET 20",
		"WHERE NOT (Id IN ($1, $2, $3)) LIMIT 1",
		"SELECT SUM(Total) FROM Orders",
	} {
		if !scannertest.HasSQLContaining(stmts, want) {
			t.Fatalf("expected synthetic SQL containing %q, got %+v", want, stmts)
		}
	}
}

func TestLINQTableMappingResolution(t *testing.T) {
	src := `using System.ComponentModel.DataAnnotations.Schema;

[Table("app_users", Schema = "crm")]
class User {}
class Order {}
class MyDbContext { public object Users { get; set; } public object Orders { get; set; } }
class Repo {
    void Configure(ModelBuilder modelBuilder) {
        modelBuilder.Entity<Order>().ToTable("sales_orders");
    }
    void Run(MyDbContext db) {
        db.Set<User>().Count();
        db.Set<Order>().Count();
    }
}`
	stmts := scanCS(t, src)
	if !scannertest.HasSQLContaining(stmts, "SELECT COUNT(*) FROM crm.app_users") {
		t.Fatalf("expected Table attribute mapping, got %+v", stmts)
	}
	if !scannertest.HasSQLContaining(stmts, "SELECT COUNT(*) FROM sales_orders") {
		t.Fatalf("expected ToTable mapping, got %+v", stmts)
	}
}

func TestRunRoslynExtractorTimeout(t *testing.T) {
	t.Setenv("VALK_FAKE_DOTNET", "sleep")
	s := &Scanner{DotnetPath: os.Args[0], ProjectPath: "fake.csproj", Timeout: 20 * time.Millisecond}
	ctx := context.Background()
	_, err := s.runRoslynExtractor(ctx, []string{"fake.cs"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRunRoslynExtractorMalformedJSON(t *testing.T) {
	t.Setenv("VALK_FAKE_DOTNET", "malformed-json")
	s := &Scanner{DotnetPath: os.Args[0], ProjectPath: "fake.csproj", Timeout: 10 * time.Second}
	_, err := s.runRoslynExtractor(context.Background(), []string{"fake.cs"})
	if err == nil || !strings.Contains(err.Error(), "decode C# Roslyn extractor output") {
		t.Fatalf("expected malformed JSON error, got %v", err)
	}
}
