# Output Formats

Valk Guard supports three output formats:

1. `terminal` (human-readable plain text)
2. `json` (machine-readable findings)
3. `sarif` (SARIF 2.1.0 for code-scanning integrations)

## Terminal

Default format:

```bash
valk-guard scan .
```

Example:

```text
db/query.sql:10: warning [VG001] avoid SELECT *; project only required columns

1 finding
Suppress findings with:
  SQL: -- valk-guard:disable <RULE_ID>
  Go:  // valk-guard:disable <RULE_ID>
  Py:  # valk-guard:disable <RULE_ID>
```

## JSON

Use:

```bash
valk-guard scan . --format json
```

JSON output is a versioned envelope:

- `version` (`string`)
- `findings` (`array`)
- `summary.total` (`number`)

Each item in `findings` contains:

- `rule_id` (`string`)
- `severity` (`"error" | "warning" | "info"`)
- `message` (`string`)
- `file` (`string`)
- `line` (`number`)
- `column` (`number`)
- `sql` (`string`, optional)

Example:

```json
{
  "version": "1.0.0",
  "findings": [
    {
      "rule_id": "VG001",
      "severity": "warning",
      "message": "avoid SELECT *; project only required columns",
      "file": "db/query.sql",
      "line": 10,
      "column": 1,
      "sql": "SELECT * FROM users"
    }
  ],
  "summary": {
    "total": 1
  }
}
```

## SARIF

Use:

```bash
valk-guard scan . --format sarif --output valk-guard.sarif
```

SARIF output follows spec version `2.1.0` and can be uploaded to GitHub code scanning.
