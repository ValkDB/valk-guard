# GitHub PR Reviewer Mode

Valk Guard is CLI-first and can run locally with no CI integration.

GitHub reviewer mode is optional and can post inline pull-request review comments via reviewdog.
The cleanest path is `--format rdjsonl` directly into reviewdog. You can still
export raw findings as `json` for downstream tooling when needed.

For copy-paste workflows, use the dedicated docs:

- Full-repo PR scan: [`docs/ci-example-full-scan.md`](ci-example-full-scan.md)
- Changed-files-only PR scan: [`docs/ci-example-changed-files.md`](ci-example-changed-files.md)

## Required Permissions

```yaml
permissions:
  contents: read
  pull-requests: write
```

## Non-Blocking PR Comment Workflow

```yaml
- uses: reviewdog/action-setup@v1

- name: Run Valk Guard on changed files
  id: scan
  run: |
    valk-guard scan "${files[@]}" --format rdjsonl > valk-guard.rdjsonl || exit_code=$?
    if [ "${exit_code:-0}" -gt 1 ]; then exit $exit_code; fi
  continue-on-error: false

- name: Post PR review comments
  env:
    REVIEWDOG_GITHUB_API_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    reviewdog \
      -f=rdjsonl \
      -name="valk-guard" \
      -reporter=github-pr-review \
      -filter-mode=added \
      -fail-level=none \
      < valk-guard.rdjsonl
```

This keeps CI non-blocking for findings (`exit 1`) while still posting review comments.
Exit code `1` (findings detected) is treated as non-fatal; only exit code `2` or higher (config/runtime error) fails the step.

For CI reproducibility, prefer pinning the install target to a version/tag instead of `@latest`.

## Full Example Workflow (Install + reviewdog + JSON + SARIF)

```yaml
name: valk-guard-pr

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  pull-requests: write
  security-events: write

env:
  # Pin in CI for stable behavior and repeatable output processing.
  VALK_GUARD_INSTALL_REF: vX.Y.Z

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25.6"

      - name: Install valk-guard
        run: go install github.com/valkdb/valk-guard/cmd/valk-guard@${VALK_GUARD_INSTALL_REF}

      - name: Collect changed files
        id: changed
        uses: tj-actions/changed-files@v45
        with:
          separator: "\n"
          files: |
            **/*.sql
            **/*.go
            **/*.py

      - uses: reviewdog/action-setup@v1

      - name: Run valk-guard (reviewdog)
        if: steps.changed.outputs.any_changed == 'true'
        id: scan_reviewdog
        run: |
          # tj-actions/changed-files with separator: '\n' writes one path per line.
          # mapfile -t reads that safely, handling spaces in filenames.
          mapfile -t files < <(printf '%s' "${{ steps.changed.outputs.all_changed_files }}")
          valk-guard scan "${files[@]}" --format rdjsonl > valk-guard.rdjsonl || exit_code=$?
          if [ "${exit_code:-0}" -gt 1 ]; then exit $exit_code; fi
        continue-on-error: false

      - name: Post PR review comments
        if: steps.changed.outputs.any_changed == 'true'
        env:
          REVIEWDOG_GITHUB_API_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          reviewdog \
            -f=rdjsonl \
            -name="valk-guard" \
            -reporter=github-pr-review \
            -filter-mode=added \
            -fail-level=none \
            < valk-guard.rdjsonl

      - name: Run valk-guard (JSON artifact)
        if: steps.changed.outputs.any_changed == 'true'
        run: |
          mapfile -t files < <(printf '%s' "${{ steps.changed.outputs.all_changed_files }}")
          valk-guard scan "${files[@]}" --format json > valk-guard.json || exit_code=$?
          if [ "${exit_code:-0}" -gt 1 ]; then exit $exit_code; fi

      - name: Upload JSON findings artifact
        if: steps.changed.outputs.any_changed == 'true'
        uses: actions/upload-artifact@v4
        with:
          name: valk-guard-pr-json-${{ github.event.pull_request.number }}
          path: valk-guard.json

      - name: Run valk-guard (SARIF)
        if: steps.changed.outputs.any_changed == 'true'
        run: |
          mapfile -t files < <(printf '%s' "${{ steps.changed.outputs.all_changed_files }}")
          valk-guard scan "${files[@]}" --format sarif --output valk-guard.sarif

      - name: Upload SARIF to code scanning
        if: steps.changed.outputs.any_changed == 'true'
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: valk-guard.sarif
```

## JSON Envelope Note (custom post-processing)

`--format json` emits a versioned envelope (`version`, `findings`, `summary`).
If you post-process with `jq`, normalize input before iterating findings:

```bash
jq -cr '((if type == "array" then . else .findings end) // [])[]'
```
## Changed-Files-Only Pattern (Recommended for PRs)

Run Valk Guard on PR-diff files (`.sql`, `.go`, `.py`) instead of full-repo scans to reduce noise and runtime.

## Resolution Model

1. Preferred: fix code and push changes; findings disappear on the next scan.
2. Export/download `valk-guard.json` from workflow artifacts when external tooling needs machine-readable results.
3. Comment-command resolution (for example `/ignore`) requires a custom bot with state management.
