# Valk Guard Examples

This directory contains examples demonstrating how Valk Guard scans different languages and patterns.

## Structure

*   `raw_sql/`: Standard `.sql` files.
*   `go_std/`: Go code using the standard `database/sql` package.
*   `go_goqu/`: Go code using the `goqu` query builder (AST analysis).
*   `python_sqlalchemy/`: Python code using SQLAlchemy ORM/Core (AST analysis).

## Running the Examples

You can run `valk-guard` against these folders to see the linter in action.

```bash
# Scan all examples
valk-guard scan examples/

# Scan a specific example
valk-guard scan examples/go_goqu/
```

## Expected Output

The linter will flag deliberate anti-patterns included in these files (e.g., `SELECT *`, missing `WHERE`, etc.). This confirms the scanner is working correctly across all supported modes.
