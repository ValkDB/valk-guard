// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package pyrunner provides shared Python subprocess plumbing used by
// packages that invoke embedded Python scripts (e.g. the SQLAlchemy
// scanner and the pymodel extractor). It centralises temp-file
// management, python3 look-up, and directory walking for .py files.
package pyrunner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WriteTempScript writes content to a temporary file with the given name
// pattern prefix and returns the file path together with a cleanup function
// that removes the file. The caller must invoke cleanup when the script is
// no longer needed.
func WriteTempScript(content []byte) (scriptPath string, cleanup func(), err error) {
	tmpScript, err := os.CreateTemp("", "valk-guard-py-*.py")
	if err != nil {
		return "", func() {}, err
	}
	removeFn := func() { _ = os.Remove(tmpScript.Name()) } //nolint:errcheck // best-effort cleanup

	if _, err := tmpScript.Write(content); err != nil {
		closeErr := tmpScript.Close()
		if closeErr != nil {
			return "", removeFn, fmt.Errorf("writing temp script: %w (close error: %v)", err, closeErr)
		}
		return "", removeFn, fmt.Errorf("writing temp script: %w", err)
	}
	if err := tmpScript.Close(); err != nil {
		return "", removeFn, err
	}

	return tmpScript.Name(), removeFn, nil
}

// ExecScript invokes python3 with the given script path and file arguments,
// returning the raw stdout bytes. It checks that python3 is available via
// LookPath, honours the context for timeouts and cancellation, and captures
// stderr for error diagnostics.
func ExecScript(ctx context.Context, scriptPath string, files []string) ([]byte, error) {
	if _, err := exec.LookPath("python3"); err != nil {
		return nil, fmt.Errorf("python3 not found; Python 3 is required for scanning .py files")
	}

	// Build command: python3 <script> file1.py file2.py ...
	args := append([]string{scriptPath}, files...)
	cmd := exec.CommandContext(ctx, "python3", args...) //nolint:gosec // args are file paths we collected

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("python script execution timed out: %w", ctx.Err())
			}
			return nil, fmt.Errorf("python script execution cancelled: %w", ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, err
		}
		return nil, fmt.Errorf("python script execution failed: %s: %w", msg, err)
	}

	return out, nil
}

// CollectPyCandidates walks the given root paths, finds .py files whose
// content contains at least one of the provided marker strings, and returns
// their paths. Files that do not contain any marker are skipped. This is
// the "quick-reject" filter that avoids unnecessary subprocess invocations.
func CollectPyCandidates(ctx context.Context, roots []string, markers []string) ([]string, error) {
	var candidates []string

	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".py" {
				return nil
			}

			data, readErr := os.ReadFile(path) //nolint:gosec // scanning user-provided source paths
			if readErr != nil {
				return fmt.Errorf("reading python file %s: %w", path, readErr)
			}

			src := string(data)
			if !containsAny(src, markers) {
				return nil
			}

			candidates = append(candidates, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return candidates, nil
}

// containsAny reports whether s contains any of the provided needle substrings.
func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
