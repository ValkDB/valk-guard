package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/valkdb/postgresparser"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/output"
	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scanner/goqu"
	"github.com/valkdb/valk-guard/internal/scanner/sqlalchemy"
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
			if coded.err != nil && coded.err.Error() != "" {
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
	noColor    bool
}

// newScanCmd constructs the "scan" subcommand with its flags and wires it to
// runScan.
func newScanCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts scanOptions

	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan source files for SQL anti-patterns",
		RunE: func(_ *cobra.Command, args []string) error {
			return runScan(opts, args, stdout, stderr)
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to config file (default: .valk-guard.yaml or .valk-guard.yml)")
	cmd.Flags().StringVar(&opts.format, "format", "", "Output format: terminal, json, sarif")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Write report to file instead of stdout")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "Disable terminal colors")

	return cmd
}

// runScan loads configuration, resolves the output reporter, collects source
// files from the given paths, runs all enabled rules, and writes the report.
// It returns a codedError so the caller can propagate the correct exit code.
func runScan(opts scanOptions, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	format := cfg.Format
	if opts.format != "" {
		format = opts.format
	}
	if format == "" {
		format = config.Default().Format
	}

	reporter, err := buildReporter(format, opts.noColor)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	logger, err := buildLogger(stderr, opts.logLevel)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	out := stdout
	if opts.outputPath != "" {
		f, createErr := os.Create(opts.outputPath)
		if createErr != nil {
			return &codedError{
				code: exitError,
				err:  fmt.Errorf("creating output file %s: %w", opts.outputPath, createErr),
			}
		}
		defer f.Close() //nolint:errcheck // best-effort close after reporting
		out = f
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	findings, err := collectAndAnalyze(ctx, args, cfg, logger)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	if err := reporter.Report(ctx, out, findings); err != nil {
		return &codedError{code: exitError, err: err}
	}

	if len(findings) > 0 {
		return &codedError{code: exitFindings}
	}
	return nil
}

// namedScanner associates a human-readable scanner name with its implementation
// and the set of input files it should process.
type namedScanner struct {
	name string
	impl scanner.Scanner
	in   []string
}

// scannerInputs groups the file paths collected during directory walking by
// their language / file type.
type scannerInputs struct {
	sqlFiles []string
	goFiles  []string
	pyFiles  []string
}

// plannedRules organises enabled rules into two buckets: those that apply to
// every SQL command type (any) and those indexed by a specific command type
// (byCommand), enabling fast dispatch during analysis.
type plannedRules struct {
	any       []rules.Rule
	byCommand map[postgresparser.QueryCommand][]rules.Rule
}

// collectAndAnalyze walks the given paths to discover source files, fans out
// scanning across all registered scanners concurrently, parses each SQL
// statement, applies enabled rules, and returns the deduplicated, sorted
// findings.
func collectAndAnalyze(ctx context.Context, paths []string, cfg *config.Config, logger *slog.Logger) ([]rules.Finding, error) {
	inputs, err := collectScannerInputs(ctx, paths, cfg)
	if err != nil {
		return nil, err
	}

	active := activeScanners(inputs, logger)
	if len(active) == 0 {
		logger.Debug("no candidate files found")
		return nil, nil
	}

	rulePlan := buildRulePlan(cfg, rules.DefaultRegistry())

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		firstErr error
		errMu    sync.Mutex
	)
	setFirstErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	results := fanOutScanners(scanCtx, active, logger, setFirstErr)

	var findings []rules.Finding
	statements := 0

	for stmt := range results {
		errMu.Lock()
		activeErr := firstErr
		errMu.Unlock()
		if activeErr != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			setFirstErr(err)
			continue
		}

		if cfg.ShouldExclude(stmt.File) {
			logger.Debug("statement excluded by config", "file", stmt.File)
			continue
		}

		statements++
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			setFirstErr(fmt.Errorf("parse error at %s:%d: %w", stmt.File, stmt.Line, err))
			continue
		}
		if parsed == nil {
			continue
		}

		applyRule := func(rule rules.Rule) {
			if !cfg.IsRuleEnabledForEngine(rule.ID(), stmt.Engine) {
				return
			}
			if scanner.IsDisabled(rule.ID(), stmt.Disabled) {
				return
			}
			ruleFindings := rule.Check(parsed, stmt.File, stmt.Line, stmt.SQL)
			for i := range ruleFindings {
				ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
			}
			findings = append(findings, ruleFindings...)
		}

		for _, rule := range rulePlan.byCommand[parsed.Command] {
			applyRule(rule)
		}
		for _, rule := range rulePlan.any {
			applyRule(rule)
		}
	}

	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return nil, firstErr
	}

	sortFindings(findings)

	logger.Debug("analysis complete", "statements", statements, "findings", len(findings))
	return findings, nil
}

// activeScanners builds the full list of named scanners from the collected
// inputs, filters out any scanner with no candidate files, and logs the
// outcome for each scanner.
func activeScanners(inputs scannerInputs, logger *slog.Logger) []namedScanner {
	all := []namedScanner{
		{name: "sql", impl: &scanner.RawSQLScanner{}, in: inputs.sqlFiles},
		{name: "go", impl: &scanner.GoScanner{}, in: inputs.goFiles},
		{name: "goqu", impl: &goqu.Scanner{}, in: inputs.goFiles},
		{name: "sqlalchemy", impl: &sqlalchemy.Scanner{}, in: inputs.pyFiles},
	}

	active := make([]namedScanner, 0, len(all))
	for _, sc := range all {
		if len(sc.in) == 0 {
			logger.Debug("scanner skipped", "scanner", sc.name, "files", 0)
			continue
		}
		logger.Debug("scanner queued", "scanner", sc.name, "files", len(sc.in))
		active = append(active, sc)
	}
	return active
}

// fanOutScanners launches each scanner in its own goroutine, writing
// discovered statements into a returned channel. The channel is closed once
// all goroutines finish. Any scanner error is reported via setFirstErr and
// causes the scan context to be cancelled.
func fanOutScanners(
	scanCtx context.Context,
	active []namedScanner,
	logger *slog.Logger,
	setFirstErr func(error),
) <-chan scanner.SQLStatement {
	results := make(chan scanner.SQLStatement, 256)
	var wg sync.WaitGroup

	for _, sc := range active {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					setFirstErr(fmt.Errorf("scanner %s panicked: %v", sc.name, r))
				}
			}()

			emitted := 0
			for stmt, err := range sc.impl.Scan(scanCtx, sc.in) {
				if err != nil {
					setFirstErr(fmt.Errorf("scanner %s: %w", sc.name, err))
					return
				}

				emitted++
				select {
				case results <- stmt:
				case <-scanCtx.Done():
					return
				}
			}

			logger.Debug("scanner completed", "scanner", sc.name, "statements", emitted)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// sortFindings sorts findings deterministically by file, line, column,
// rule ID, and message so that report output is stable across runs.
func sortFindings(findings []rules.Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		if findings[i].Column != findings[j].Column {
			return findings[i].Column < findings[j].Column
		}
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Message < findings[j].Message
	})
}

// buildRulePlan iterates over the registry and partitions enabled rules by the
// SQL command types they target, producing a plannedRules ready for fast
// per-statement dispatch.
func buildRulePlan(cfg *config.Config, reg *rules.Registry) plannedRules {
	plan := plannedRules{
		byCommand: make(map[postgresparser.QueryCommand][]rules.Rule),
	}

	for _, rule := range reg.All() {
		if !cfg.IsRuleEnabled(rule.ID()) {
			continue
		}
		targets := ruleCommandTargets(rule.ID())
		if len(targets) == 0 {
			plan.any = append(plan.any, rule)
			continue
		}
		for _, command := range targets {
			plan.byCommand[command] = append(plan.byCommand[command], rule)
		}
	}

	return plan
}

// ruleCommandTargets returns the SQL command types that a given rule targets.
// Rules that apply to all command types return nil.
func ruleCommandTargets(ruleID string) []postgresparser.QueryCommand {
	switch ruleID {
	case "VG001", "VG004", "VG006":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandSelect}
	case "VG002":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandUpdate}
	case "VG003":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandDelete}
	case "VG007", "VG008":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandDDL}
	default:
		// Cross-cutting rules (for example VG005) run for all command types.
		return nil
	}
}

// collectScannerInputs recursively walks the provided paths, classifying each
// file by extension into SQL, Go, or Python buckets while respecting config
// exclusion patterns and context cancellation.
func collectScannerInputs(ctx context.Context, paths []string, cfg *config.Config) (scannerInputs, error) {
	var inputs scannerInputs
	seenSQL := make(map[string]struct{})
	seenGo := make(map[string]struct{})
	seenPy := make(map[string]struct{})

	add := func(dst *[]string, seen map[string]struct{}, path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		*dst = append(*dst, path)
	}

	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return scannerInputs{}, err
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if cfg.ShouldExclude(path) {
				return nil
			}

			switch strings.ToLower(filepath.Ext(path)) {
			case ".sql":
				add(&inputs.sqlFiles, seenSQL, path)
			case ".go":
				add(&inputs.goFiles, seenGo, path)
			case ".py":
				add(&inputs.pyFiles, seenPy, path)
			}
			return nil
		})
		if err != nil {
			return scannerInputs{}, fmt.Errorf("walking %s: %w", root, err)
		}
	}

	sort.Strings(inputs.sqlFiles)
	sort.Strings(inputs.goFiles)
	sort.Strings(inputs.pyFiles)
	return inputs, nil
}

// buildReporter constructs the output.Reporter corresponding to the requested
// format string (config.FormatTerminal, config.FormatJSON, or config.FormatSARIF).
func buildReporter(format string, noColor bool) (output.Reporter, error) {
	switch format {
	case config.FormatTerminal:
		return &output.TerminalReporter{NoColor: noColor}, nil
	case config.FormatJSON:
		return &output.JSONReporter{}, nil
	case config.FormatSARIF:
		return &output.SARIFReporter{Version: version}, nil
	default:
		return nil, fmt.Errorf("invalid format %q: must be terminal, json, or sarif", format)
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
