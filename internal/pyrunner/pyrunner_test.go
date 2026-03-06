// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package pyrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTempScript(t *testing.T) {
	content := []byte("print('hello')")
	path, cleanup, err := WriteTempScript(content)
	if err != nil {
		t.Fatalf("WriteTempScript: %v", err)
	}
	defer cleanup()

	if path == "" {
		t.Fatal("expected non-empty script path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading temp script: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("expected %q, got %q", content, data)
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected temp file to be removed after cleanup")
	}
}

func TestExecScriptHelloWorld(t *testing.T) {
	script := []byte(`import json, sys; json.dump(["ok"], sys.stdout)`)
	path, cleanup, err := WriteTempScript(script)
	if err != nil {
		t.Fatalf("WriteTempScript: %v", err)
	}
	defer cleanup()

	out, err := ExecScript(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	if string(out) != `["ok"]` {
		t.Fatalf("expected [\"ok\"], got %q", out)
	}
}

func TestExecScriptPassesFileArgs(t *testing.T) {
	script := []byte(`import json, sys; json.dump(sys.argv[1:], sys.stdout)`)
	path, cleanup, err := WriteTempScript(script)
	if err != nil {
		t.Fatalf("WriteTempScript: %v", err)
	}
	defer cleanup()

	out, err := ExecScript(context.Background(), path, []string{"a.py", "b.py"})
	if err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	if string(out) != `["a.py", "b.py"]` {
		t.Fatalf("expected file args, got %q", out)
	}
}

func TestExecScriptContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	script := []byte(`import time; time.sleep(10)`)
	path, cleanup, err := WriteTempScript(script)
	if err != nil {
		t.Fatalf("WriteTempScript: %v", err)
	}
	defer cleanup()

	_, err = ExecScript(ctx, path, nil)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestExecScriptSyntaxError(t *testing.T) {
	script := []byte(`this is not valid python`)
	path, cleanup, err := WriteTempScript(script)
	if err != nil {
		t.Fatalf("WriteTempScript: %v", err)
	}
	defer cleanup()

	_, err = ExecScript(context.Background(), path, nil)
	if err == nil {
		t.Fatal("expected error for invalid Python script")
	}
}

func TestCollectPyCandidates(t *testing.T) {
	tmpDir := t.TempDir()

	// File with marker
	if err := os.WriteFile(filepath.Join(tmpDir, "has_sa.py"), []byte("from sqlalchemy import select\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// File without marker
	if err := os.WriteFile(filepath.Join(tmpDir, "no_sa.py"), []byte("print('hello')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-Python file
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("sqlalchemy\n"), 0644); err != nil {
		t.Fatal(err)
	}

	candidates, err := CollectPyCandidates(context.Background(), []string{tmpDir}, []string{"sqlalchemy"})
	if err != nil {
		t.Fatalf("CollectPyCandidates: %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	if filepath.Base(candidates[0]) != "has_sa.py" {
		t.Fatalf("expected has_sa.py, got %s", candidates[0])
	}
}

func TestCollectPyCandidatesEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	candidates, err := CollectPyCandidates(context.Background(), []string{tmpDir}, []string{"sqlalchemy"})
	if err != nil {
		t.Fatalf("CollectPyCandidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello sqlalchemy world", []string{"sqlalchemy"}) {
		t.Fatal("expected match")
	}
	if containsAny("hello world", []string{"sqlalchemy", "django"}) {
		t.Fatal("expected no match")
	}
	if containsAny("", []string{"anything"}) {
		t.Fatal("expected no match on empty string")
	}
}
