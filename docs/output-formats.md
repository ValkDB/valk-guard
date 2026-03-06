# Output Formats

Valk Guard supports four output formats:

1. `terminal` (human-readable plain text)
2. `json` (machine-readable findings)
3. `rdjsonl` (reviewdog-compatible diagnostics for PR comments)
4. `sarif` (SARIF 2.1.0 for code-scanning integrations)

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

## RDJSONL

Use:

```bash
valk-guard scan . --format rdjsonl
```

`rdjsonl` is designed for `reviewdog -f=rdjsonl`. Each finding is emitted as one
JSON object per line with:

- reviewdog severity (`ERROR`, `WARNING`, `INFO`)
- rule code (`VG001`, `VG004`, ...)
- inline location (`path`, `line`, `column`)
- a cleaned message with:
  - the valk-guard rule message
  - a human-readable origin hint for synthetic builder-derived SQL
  - a compact query preview

Synthetic scanner prefixes such as `/* valk-guard:synthetic sqlalchemy-ast */`
are stripped automatically from the user-facing message.

Example:

```json
{"source":{"name":"valk-guard","url":"https://github.com/ValkDB/valk-guard"},"severity":"WARNING","code":{"value":"VG004"},"message":"VG004: SELECT without LIMIT may return unbounded rows; add LIMIT or FETCH FIRST | Origin: SQLAlchemy query builder | Query: `SELECT \"User\".\"id\" FROM \"User\"`","location":{"path":"app/queries.py","range":{"start":{"line":23,"column":1},"end":{"line":23,"column":2}}}}
```

## SARIF

Use:

```bash
valk-guard scan . --format sarif --output valk-guard.sarif
```

SARIF output follows spec version `2.1.0` and can be uploaded to GitHub code scanning.
