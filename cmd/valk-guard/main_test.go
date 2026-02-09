package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScanJSONFindingsExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "query.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT * FROM users;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitFindings, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"rule_id"`) {
		t.Fatalf("expected JSON findings output, got: %s", stdout.String())
	}
}

func TestRunScanParseErrorExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "broken.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT FROM;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("expected exit code %d, got %d", exitError, code)
	}
	if !strings.Contains(stderr.String(), "parse error at") {
		t.Fatalf("expected parse error message, got %q", stderr.String())
	}
}

func TestRunScanExcludedBrokenGoFileDoesNotFail(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte("exclude:\n  - broken.go\n"), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "broken.go"), []byte("this is not valid go"), 0644); err != nil {
		t.Fatalf("failed to write go fixture: %v", err)
	}
	// Clean SQL that should not trigger rules.
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitSuccess, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunScanEmptyConfigFormatFallsBackToTerminal(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte("format: \"\"\n"), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitSuccess, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0 findings") {
		t.Fatalf("expected terminal output fallback, got %q", stdout.String())
	}
}

func TestRunScanInvalidLogLevelExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--log-level", "trace"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("expected exit code %d, got %d", exitError, code)
	}
	if !strings.Contains(stderr.String(), "invalid log level") {
		t.Fatalf("expected invalid log level message, got %q", stderr.String())
	}
}
