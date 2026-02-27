# GitHub PR Reviewer Mode

Valk Guard is CLI-first and can run locally with no CI integration.

GitHub reviewer mode is optional and can post inline pull-request review comments via reviewdog.
The CI job also exports raw findings as a JSON artifact (`valk-guard.json`) for downstream use.

## Required Permissions

```yaml
permissions:
  contents: read
  pull-requests: write
```

## Non-Blocking PR Comment Workflow

```yaml
- name: Run Valk Guard on changed files
  run: ./valk-guard scan "${files[@]}" --format json > valk-guard.json

- name: Upload JSON findings artifact
  uses: actions/upload-artifact@v4
  with:
    name: valk-guard-pr-json-${{ github.event.pull_request.number }}
    path: valk-guard.json
```

This keeps CI non-blocking for findings (`exit 1`) while still posting review comments and preserving machine-readable output.

## Changed-Files-Only Pattern (Recommended for PRs)

Run Valk Guard on PR-diff files (`.sql`, `.go`, `.py`) instead of full-repo scans to reduce noise and runtime.

## Resolution Model

1. Preferred: fix code and push changes; findings disappear on the next scan.
2. Export/download `valk-guard.json` from workflow artifacts when external tooling needs machine-readable results.
3. Comment-command resolution (for example `/ignore`) requires a custom bot with state management.
