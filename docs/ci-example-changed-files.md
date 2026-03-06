## CI Example: Changed-Files-Only PR Scan

Copy-paste this when you want PR scans to look only at changed `.sql`, `.go`, and `.py` files.

Use it when:

- the repo is larger
- you want faster PR runs
- you want a cleaner workflow than hand-written `git diff` shell logic

```yaml
name: valk-guard-pr

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  pull-requests: write

env:
  VALK_GUARD_INSTALL_REF: vX.Y.Z

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
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
        if: steps.changed.outputs.any_changed == 'true'

      - name: Run valk-guard
        if: steps.changed.outputs.any_changed == 'true'
        run: |
          mapfile -t files < <(printf '%s' "${{ steps.changed.outputs.all_changed_files }}")

          set +e
          valk-guard scan "${files[@]}" --config .valk-guard.yaml --format rdjsonl > valk-guard.rdjsonl
          code=$?
          set -e

          if [ "$code" -gt 1 ]; then
            exit "$code"
          fi

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
```

Install choices:

- local dev: `go install github.com/valkdb/valk-guard/cmd/valk-guard@latest`
- CI release pin: `go install github.com/valkdb/valk-guard/cmd/valk-guard@vX.Y.Z`
- CI commit pin: `go install github.com/valkdb/valk-guard/cmd/valk-guard@<commit-sha>`
