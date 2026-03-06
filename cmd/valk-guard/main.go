// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/output"
	"github.com/valkdb/valk-guard/internal/rules"
)

const (
	exitSuccess  = 0
	exitFindings = 1
	exitError    = 2
)

var version = "dev"

// codedError pairs an exit code with an underlying error so that the top-level
// run function can propagate both to the OS and to the user.
type codedError struct {
	code int
	err  error
}

// Error implements the error interface, returning the underlying error message.
func (e *codedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap returns the underlying error for errors.Is/errors.As support.
func (e *codedError) Unwrap() error {
	return e.err
}

// main is the entry point for the valk-guard binary.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run builds and executes the root cobra command, returning the appropriate
// OS exit code based on the outcome.
func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var coded *codedError
		if errors.As(err, &coded) {
			if coded.err != nil {
				_, _ = fmt.Fprintln(stderr, coded.err.Error())
			}
			return coded.code
		}
		_, _ = fmt.Fprintln(stderr, err.Error())
		return exitError
	}
	return exitSuccess
}

// newRootCmd constructs the top-level cobra command and registers all
// subcommands.
func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "valk-guard",
		Short:         "Static SQL performance linter for CI/CD",
		Long:          "Valk Guard scans SQL across SQL, Go, Goqu, and SQLAlchemy sources and reports performance, safety, and schema-aware findings.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.AddCommand(newScanCmd(stdout, stderr))
	return cmd
}

// scanOptions holds the CLI flag values parsed for the scan subcommand.
type scanOptions struct {
	configPath string
	format     string
	outputPath string
	logLevel   string
}

// newScanCmd constructs the "scan" subcommand with its flags and wires it to
// runScan.
func newScanCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts scanOptions

	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan source files for SQL anti-patterns",
		Long: strings.Join([]string{
			"Scan one or more paths for SQL findings.",
			"",
			"Exit codes:",
			"  0: no findings",
			"  1: findings detected",
			"  2: config/runtime error",
		}, "\n"),
		Example: strings.Join([]string{
			"  valk-guard scan .",
			"  valk-guard scan ./queries --format json",
			"  valk-guard scan . --format rdjsonl",
			"  valk-guard scan . --format sarif --output results.sarif",
			"  valk-guard scan . --config .valk-guard.yaml",
		}, "\n"),
		RunE: func(_ *cobra.Command, args []string) error {
			code, err := runScan(opts, args, stdout, stderr)
			if err != nil {
				return &codedError{code: code, err: err}
			}
			if code != exitSuccess {
				return &codedError{code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to config file (default: .valk-guard.yaml or .valk-guard.yml)")
	cmd.Flags().StringVar(&opts.format, "format", "", "Output format: terminal (default), json, rdjsonl, sarif")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Write report to file instead of stdout")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")

	return cmd
}

// runScan loads configuration, resolves the output reporter, collects source
// files from the given paths, runs all enabled rules, and writes the report.
// It returns an exit code and an optional error. Exit code 1 means findings
// were detected (not an error); exit code 2 means a real error occurred.
func runScan(opts scanOptions, args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		args = []string{"."}
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return exitError, err
	}

	format := cfg.Format
	if opts.format != "" {
		format = opts.format
	}
	if format == "" {
		format = config.Default().Format
	}

	reporter, err := buildReporter(format)
	if err != nil {
		return exitError, err
	}

	logger, err := buildLogger(stderr, opts.logLevel)
	if err != nil {
		return exitError, err
	}

	reg := rules.DefaultRegistry()
	warnUnknownConfiguredRules(cfg, reg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	findings, hadScannableInputs, err := collectAndAnalyze(ctx, args, cfg, reg, logger)
	if err != nil {
		return exitError, err
	}
	if !hadScannableInputs {
		logger.Warn("no .sql, .go, or .py files found in scan paths")
	}

	out := stdout
	var tmpPath string
	if opts.outputPath != "" {
		dir := filepath.Dir(opts.outputPath)
		tmpFile, createErr := os.CreateTemp(dir, ".valk-guard-*.tmp")
		if createErr != nil {
			return exitError, fmt.Errorf("creating output file %s: %w", opts.outputPath, createErr)
		}
		tmpPath = tmpFile.Name()
		out = tmpFile
		defer func() {
			_ = tmpFile.Close()    // best-effort close
			_ = os.Remove(tmpPath) // clean up temp file; no-op after successful rename
		}()
	}

	if err := reporter.Report(ctx, out, findings); err != nil {
		return exitError, err
	}

	// Print "no scannable files" hint after the report for terminal output
	// so it appears below the "0 findings" line.
	if !hadScannableInputs && format == config.FormatTerminal && len(findings) == 0 {
		_, _ = fmt.Fprintln(out, "(no scannable files found)")
	}

	if opts.outputPath != "" {
		if f, ok := out.(*os.File); ok {
			if err := f.Close(); err != nil {
				return exitError, fmt.Errorf("closing output file: %w", err)
			}
			if err := os.Rename(tmpPath, opts.outputPath); err != nil {
				return exitError, fmt.Errorf("writing output file %s: %w", opts.outputPath, err)
			}
		}
	}

	if len(findings) > 0 {
		return exitFindings, nil
	}
	return exitSuccess, nil
}

// buildReporter constructs the output.Reporter corresponding to the requested
// format string (config.FormatTerminal, config.FormatJSON, config.FormatRDJSONL,
// or config.FormatSARIF).
func buildReporter(format string) (output.Reporter, error) {
	switch format {
	case config.FormatTerminal:
		return &output.TerminalReporter{}, nil
	case config.FormatJSON:
		return &output.JSONReporter{Version: version}, nil
	case config.FormatRDJSONL:
		return &output.RDJSONLReporter{}, nil
	case config.FormatSARIF:
		return &output.SARIFReporter{Version: version}, nil
	default:
		return nil, fmt.Errorf("invalid format %q: must be terminal, json, rdjsonl, or sarif", format)
	}
}

// buildLogger creates a structured slog.Logger that writes text-format log
// records to w at the requested level ("debug", "info", "warn", or "error").
func buildLogger(w io.Writer, level string) (*slog.Logger, error) {
	if level == "" {
		level = "warn"
	}

	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q: must be debug, info, warn, or error", level)
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(handler), nil
}
