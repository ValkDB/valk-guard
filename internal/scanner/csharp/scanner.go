// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package csharp provides an EF Core scanner for C# source files.
//
// The scanner intentionally avoids Go-side string heuristics for C# code
// constructs. It sends readable .cs files to the embedded Roslyn extractor,
// which parses real C# syntax and decides whether raw EF Core SQL or
// deterministic DbSet/LINQ query chains are present. Extracted and synthetic
// SQL then flows through the normal Valk Guard SQL parser and rule engine.
package csharp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/valkdb/valk-guard/internal/scanner"
)

//go:embed roslynextractor/RoslynExtractor.csproj roslynextractor/Program.cs
var roslynProject embed.FS

const (
	csprojEmbedPath       = "roslynextractor/RoslynExtractor.csproj"
	programEmbedPath      = "roslynextractor/Program.cs"
	extractorTimeout      = 5 * time.Minute
	missingDotnetErrorMsg = ".NET SDK is required for scanning C# EF Core files"
)

var (
	roslynBuildMu sync.Mutex
	userCacheDir  = os.UserCacheDir
)

// Scanner extracts raw and synthetic SQL from EF Core usage in C# source files.
type Scanner struct {
	// DotnetPath optionally overrides the dotnet executable used to run the
	// embedded Roslyn extractor. Empty means PATH lookup for "dotnet".
	DotnetPath string
	// ProjectPath optionally points at a RoslynExtractor.csproj on disk. Empty
	// means the embedded extractor project is materialized and reused from the
	// user cache directory.
	ProjectPath string
	// Timeout optionally overrides the Roslyn extractor timeout. Empty uses the
	// package default. Tests use this to exercise timeout handling quickly.
	Timeout time.Duration
}

// Scan walks the given paths, sends readable C# files to the Roslyn AST
// extractor, and streams extracted SQL statements. The .NET SDK is only
// required when at least one .cs file is present and the C# source is enabled.
func (s *Scanner) Scan(ctx context.Context, paths []string) iter.Seq2[scanner.SQLStatement, error] {
	return func(yield func(scanner.SQLStatement, error) bool) {
		candidates, err := collectCandidates(ctx, paths)
		if err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}
		if len(candidates) == 0 {
			return
		}

		extracted, err := s.runRoslynExtractor(ctx, candidates)
		if err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}

		yieldWithDirectives(ctx, extracted, yield)
	}
}

// csResult represents one SQL statement emitted by the Roslyn extractor.
type csResult struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line"`
	EndColumn int    `json:"end_column"`
	SQL       string `json:"sql"`
}

// collectCandidates returns readable .cs files under paths. Candidate
// classification happens inside Roslyn so scanner logic does not depend on
// substring checks over source text.
func collectCandidates(ctx context.Context, paths []string) ([]string, error) {
	var candidates []string
	seen := make(map[string]struct{})

	err := walkCSFiles(ctx, paths, func(path string, _ []byte) error {
		clean := filepath.Clean(path)
		if _, exists := seen[clean]; exists {
			return nil
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

// walkCSFiles visits every readable .cs file under paths and passes its content
// to fn. Missing roots are ignored to match the existing scanner behavior.
func walkCSFiles(ctx context.Context, paths []string, fn func(string, []byte) error) error {
	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}

		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if !isCSharpSource(root) {
				continue
			}
			data, readErr := os.ReadFile(root) //nolint:gosec // scanner input is user-selected source code
			if readErr != nil {
				return fmt.Errorf("read C# file %s: %w", root, readErr)
			}
			if err := fn(root, data); err != nil {
				return err
			}
			continue
		}

		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !isCSharpSource(path) {
				return nil
			}
			data, readErr := os.ReadFile(path) //nolint:gosec // scanner input is user-selected source code
			if readErr != nil {
				return fmt.Errorf("read C# file %s: %w", path, readErr)
			}
			return fn(path, data)
		}); err != nil {
			return err
		}
	}
	return nil
}

// isCSharpSource reports whether path has a .cs extension.
func isCSharpSource(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".cs")
}

// runRoslynExtractor runs the Roslyn extractor and decodes the JSON statement
// stream. The embedded project is cached and published once per content hash;
// explicit ProjectPath overrides keep direct dotnet run behavior for tests and
// local extractor development.
func (s *Scanner) runRoslynExtractor(parent context.Context, files []string) ([]csResult, error) {
	project, err := s.roslynProject()
	if err != nil {
		return nil, err
	}
	defer project.cleanup()

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = extractorTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	if project.cached {
		if _, err := os.Stat(project.executablePath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("stat cached C# Roslyn extractor binary: %w", err)
			}
			dotnet, err := s.dotnetExecutable()
			if err != nil {
				return nil, err
			}
			if err := ensureCachedRoslynExtractorPublished(ctx, dotnet, project.projectPath, project.executablePath); err != nil {
				return nil, err
			}
		}
		return runRoslynBinary(ctx, project.executablePath, files, timeout)
	}

	dotnet, err := s.dotnetExecutable()
	if err != nil {
		return nil, err
	}
	return runRoslynProject(ctx, dotnet, project.projectPath, files, timeout)
}

// runRoslynProject executes an explicit extractor project with dotnet run.
func runRoslynProject(ctx context.Context, dotnet, projectPath string, files []string, timeout time.Duration) ([]csResult, error) {
	args := make([]string, 0, 4+len(files))
	args = append(args, "run", "--project", projectPath, "--")
	args = append(args, files...)
	//nolint:gosec // dotnet is a resolved SDK executable and args point at scanner-selected source files.
	//nolint:gosec // dotnet is a resolved SDK executable used to publish scanner-owned embedded code.
	//nolint:gosec // dotnet is a resolved SDK executable used to publish scanner-owned embedded code.
	cmd := exec.CommandContext(ctx, dotnet, args...)
	cmd.Env = dotnetEnv()
	return decodeRoslynCommand(cmd, ctx, timeout)
}

// runRoslynBinary executes the cached published extractor binary.
func runRoslynBinary(ctx context.Context, executablePath string, files []string, timeout time.Duration) ([]csResult, error) {
	//nolint:gosec // executablePath is the scanner-owned cached extractor binary.
	cmd := exec.CommandContext(ctx, executablePath, files...)
	return decodeRoslynCommand(cmd, ctx, timeout)
}

// decodeRoslynCommand runs cmd and decodes the extractor JSON result stream.
func decodeRoslynCommand(cmd *exec.Cmd, ctx context.Context, timeout time.Duration) ([]csResult, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("csharp Roslyn extractor timed out after %s", timeout)
		}
		return nil, fmt.Errorf("csharp Roslyn extractor failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var results []csResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("decode C# Roslyn extractor output: %w", err)
	}
	return results, nil
}

// ensureCachedRoslynExtractorPublished publishes a self-contained extractor
// binary once per embedded content hash. Later scans execute the binary
// directly without dotnet restore/build/run overhead.
func ensureCachedRoslynExtractorPublished(ctx context.Context, dotnet, projectPath, executablePath string) error {
	if _, err := os.Stat(executablePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat cached C# Roslyn extractor binary: %w", err)
	}

	roslynBuildMu.Lock()
	defer roslynBuildMu.Unlock()

	if _, err := os.Stat(executablePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat cached C# Roslyn extractor binary: %w", err)
	}

	publishDir := filepath.Dir(executablePath)
	if err := os.MkdirAll(publishDir, 0o700); err != nil {
		return fmt.Errorf("create cached C# Roslyn extractor publish dir: %w", err)
	}

	rid, err := dotnetRuntimeIdentifier()
	if err != nil {
		return err
	}

	args := []string{
		"publish", pathForDotnet(dotnet, projectPath),
		"-c", "Release",
		"-r", rid,
		"--self-contained", "true",
		"-p:PublishSingleFile=true",
		"-p:PublishTrimmed=false",
		"-o", pathForDotnet(dotnet, publishDir),
		"--nologo",
	}
	//nolint:gosec // dotnet is a resolved SDK executable used to publish scanner-owned embedded code.
	cmd := exec.CommandContext(ctx, dotnet, args...)
	cmd.Env = dotnetEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("csharp Roslyn extractor publish timed out")
		}
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return fmt.Errorf("publish cached C# Roslyn extractor: %w: %s", err, output)
	}
	if _, err := os.Stat(executablePath); err != nil {
		return fmt.Errorf("published C# Roslyn extractor binary missing at %s: %w", executablePath, err)
	}
	//nolint:gosec // cached self-contained binary must be executable by the scanner.
	if err := os.Chmod(executablePath, 0o700); err != nil {
		return fmt.Errorf("mark cached C# Roslyn extractor executable: %w", err)
	}
	return nil
}

// dotnetRuntimeIdentifier returns the Runtime Identifier used to publish the
// cached extractor for the current OS and architecture.
func dotnetRuntimeIdentifier() (string, error) {
	arch := map[string]string{
		"amd64": "x64",
		"arm64": "arm64",
	}[runtime.GOARCH]
	if arch == "" {
		return "", fmt.Errorf("unsupported architecture for C# Roslyn extractor publish: %s", runtime.GOARCH)
	}

	switch runtime.GOOS {
	case "linux":
		return "linux-" + arch, nil
	case "darwin":
		return "osx-" + arch, nil
	case "windows":
		return "win-" + arch, nil
	default:
		return "", fmt.Errorf("unsupported OS for C# Roslyn extractor publish: %s", runtime.GOOS)
	}
}

// pathForDotnet converts WSL /mnt/<drive>/... paths to Windows paths when the
// selected dotnet executable is Windows dotnet. Linux dotnet must keep POSIX
// paths unchanged.
func pathForDotnet(dotnet, path string) string {
	resolved := dotnet
	if realPath, err := filepath.EvalSymlinks(dotnet); err == nil {
		resolved = realPath
	}
	if runtime.GOOS != "linux" || !strings.HasPrefix(filepath.ToSlash(resolved), "/mnt/") {
		return path
	}

	slashPath := filepath.ToSlash(path)
	parts := strings.SplitN(strings.TrimPrefix(slashPath, "/mnt/"), "/", 2)
	if len(parts) != 2 || len(parts[0]) != 1 {
		return path
	}
	return strings.ToUpper(parts[0]) + `:\` + strings.ReplaceAll(parts[1], "/", `\`)
}

// cachedExtractorName returns the binary name emitted by dotnet publish.
func cachedExtractorName() string {
	if runtime.GOOS == "windows" {
		return "RoslynExtractor.exe"
	}
	return "RoslynExtractor"
}

type roslynRuntimeProject struct {
	projectPath    string
	executablePath string
	cached         bool
	cleanup        func()
}

// roslynProject returns a caller-provided project path or a cached copy of the
// embedded extractor keyed by the embedded project content hash.
func (s *Scanner) roslynProject() (roslynRuntimeProject, error) {
	if strings.TrimSpace(s.ProjectPath) != "" {
		return roslynRuntimeProject{projectPath: s.ProjectPath, cleanup: func() {}}, nil
	}
	cacheRoot, err := s.roslynCacheRoot()
	if err != nil {
		return roslynRuntimeProject{}, err
	}
	return cachedRoslynProject(cacheRoot)
}

// roslynCacheRoot returns a cache root visible to the dotnet process. Windows
// dotnet invoked from WSL cannot read Linux-only paths such as /home, so that
// case uses a cache location on the mounted Windows user profile.
func (s *Scanner) roslynCacheRoot() (string, error) {
	dotnetPath := strings.TrimSpace(s.DotnetPath)
	if dotnetPath == "" {
		if path, err := exec.LookPath("dotnet"); err == nil {
			dotnetPath = path
		}
	}
	if resolved, err := filepath.EvalSymlinks(dotnetPath); err == nil {
		dotnetPath = resolved
	}
	if runtime.GOOS == "linux" && strings.HasPrefix(filepath.ToSlash(dotnetPath), "/mnt/") {
		if root, ok := windowsAccessibleCacheRoot(); ok {
			return root, nil
		}
	}
	cacheRoot, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir for Roslyn extractor: %w", err)
	}
	return filepath.Join(cacheRoot, "valk-guard"), nil
}

// windowsAccessibleCacheRoot derives a Windows-profile cache path from a WSL
// checkout under /mnt/<drive>/Users/<user>/....
func windowsAccessibleCacheRoot() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(wd), "/")
	if len(parts) < 5 || parts[1] != "mnt" || !strings.EqualFold(parts[3], "users") {
		return "", false
	}
	return filepath.Clean("/" + filepath.Join(parts[1], parts[2], parts[3], parts[4], "AppData", "Local", "valk-guard")), true
}

// cachedRoslynProject materializes the embedded C# extractor under cacheRoot so
// repeated scans can execute a published binary.
func cachedRoslynProject(cacheRoot string) (roslynRuntimeProject, error) {
	csproj, program, err := embeddedRoslynFiles()
	if err != nil {
		return roslynRuntimeProject{}, err
	}

	sum := sha256.Sum256(append(append([]byte{}, csproj...), program...))
	cacheDir := filepath.Join(cacheRoot, "roslynextractor-"+hex.EncodeToString(sum[:])[:16])
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return roslynRuntimeProject{}, fmt.Errorf("create cached Roslyn extractor dir: %w", err)
	}

	projectPath := filepath.Join(cacheDir, "RoslynExtractor.csproj")
	programPath := filepath.Join(cacheDir, "Program.cs")
	if err := writeFileIfDifferent(projectPath, csproj, 0o600); err != nil {
		return roslynRuntimeProject{}, fmt.Errorf("write cached Roslyn project: %w", err)
	}
	if err := writeFileIfDifferent(programPath, program, 0o600); err != nil {
		return roslynRuntimeProject{}, fmt.Errorf("write cached Roslyn program: %w", err)
	}

	return roslynRuntimeProject{
		projectPath:    projectPath,
		executablePath: filepath.Join(cacheDir, "publish", cachedExtractorName()),
		cached:         true,
		cleanup:        func() {},
	}, nil
}

// dotnetExecutable resolves the dotnet executable, returning a scanner-specific
// error when C# candidates exist but the .NET SDK is unavailable.
func (s *Scanner) dotnetExecutable() (string, error) {
	name := strings.TrimSpace(s.DotnetPath)
	if name == "" {
		name = "dotnet"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s; install dotnet or disable the csharp source in .valk-guard.yaml", missingDotnetErrorMsg)
	}
	return path, nil
}

// dotnetEnv keeps .NET subprocesses deterministic and quiet in local scans and
// CI jobs.
func dotnetEnv() []string {
	return append(os.Environ(), "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1")
}

// embeddedRoslynFiles reads the embedded extractor project files.
func embeddedRoslynFiles() (csproj, program []byte, err error) {
	csproj, err = roslynProject.ReadFile(csprojEmbedPath)
	if err != nil {
		return nil, nil, err
	}
	program, err = roslynProject.ReadFile(programEmbedPath)
	if err != nil {
		return nil, nil, err
	}
	return csproj, program, nil
}

// writeFileIfDifferent avoids rewriting cached project files when their bytes
// have not changed, preserving incremental build outputs.
func writeFileIfDifferent(path string, data []byte, perm fs.FileMode) error {
	existing, err := os.ReadFile(path) //nolint:gosec // path is scanner-owned cache content
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// yieldWithDirectives attaches inline valk-guard disable directives from each
// C# file to the extracted SQL statements before yielding them to the engine.
func yieldWithDirectives(
	ctx context.Context,
	extracted []csResult,
	yield func(scanner.SQLStatement, error) bool,
) {
	directiveCache := make(map[string][]scanner.Directive)

	for _, e := range extracted {
		if err := ctx.Err(); err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}
		if !scanner.LooksLikeSQL(e.SQL) {
			continue
		}

		directives, ok := directiveCache[e.File]
		if !ok {
			data, err := os.ReadFile(e.File) //nolint:gosec // file was already selected for scanning
			if err != nil {
				_ = yield(scanner.SQLStatement{}, fmt.Errorf("reading C# file %s for directives: %w", e.File, err))
				return
			}
			directives = scanner.ParseDirectives(strings.Split(string(data), "\n"))
			directiveCache[e.File] = directives
		}

		if !yield(scanner.SQLStatement{
			SQL:       e.SQL,
			File:      e.File,
			Line:      e.Line,
			Column:    e.Column,
			EndLine:   e.EndLine,
			EndColumn: e.EndColumn,
			Engine:    scanner.EngineCSharp,
			Disabled:  scanner.DisabledRulesForLine(directives, e.Line),
		}, nil) {
			return
		}
	}
}
