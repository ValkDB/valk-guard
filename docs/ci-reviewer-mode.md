# GitHub PR Reviewer Mode

Valk Guard is CLI-first and can run locally with no CI integration.

GitHub reviewer mode is optional and uses SARIF so findings appear as Code Scanning annotations in pull requests.

## Required Permissions

```yaml
permissions:
  contents: read
  security-events: write
```

Add this only if you also post direct PR comments from a bot:

```yaml
permissions:
  pull-requests: write
```

## Non-Blocking SARIF Workflow

```yaml
- name: Run Valk Guard
  continue-on-error: true
  run: valk-guard scan . --format sarif --output valk-guard.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: valk-guard.sarif
```

This keeps CI green while still annotating the PR unless branch protection explicitly requires clean code scanning.

## Changed-Files-Only Pattern (Recommended for PRs)

Run Valk Guard on PR-diff files (`.sql`, `.go`, `.py`) instead of full-repo scans to reduce noise and runtime.

## Resolution Model

1. Preferred: fix code and push changes; findings disappear on the next scan.
2. Native GitHub flow: dismiss Code Scanning alerts in the UI when needed.
3. Comment-command resolution (for example `/ignore`) requires a custom bot with state management.
