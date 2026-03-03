"""Extract SQLAlchemy model definitions via Python AST analysis.

Usage: python3 extract_models.py file1.py file2.py ...
Output: JSON array of model objects.
"""

import ast
import json
import sys


def extract_models_from_file(filepath):
    """Parse a Python file and extract SQLAlchemy model definitions."""
    with open(filepath, "r", encoding="utf-8", errors="replace") as f:
        source = f.read()

    tree = ast.parse(source, filename=filepath)

    results = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.ClassDef):
            continue

        table_name = _find_tablename(node)
        if table_name is None:
            continue

        columns = _extract_columns(node)
        if not columns:
            continue

        results.append({
            "table": table_name,
            "columns": columns,
            "file": filepath,
            "line": node.lineno,
        })

    results.sort(key=lambda r: r["line"])
    return results


def _find_tablename(class_node):
    """Find __tablename__ = "..." in a class body."""
    for stmt in class_node.body:
        if not isinstance(stmt, ast.Assign):
            continue
        for target in stmt.targets:
            if isinstance(target, ast.Name) and target.id == "__tablename__":
                if isinstance(stmt.value, ast.Constant) and isinstance(stmt.value.value, str):
                    return stmt.value.value
    return None


def _extract_columns(class_node):
    """Extract Column() definitions from a class body."""
    columns = []
    for stmt in class_node.body:
        if not isinstance(stmt, ast.Assign):
            continue
        if len(stmt.targets) != 1:
            continue
        target = stmt.targets[0]
        if not isinstance(target, ast.Name):
            continue

        field_name = target.id
        if field_name.startswith("_"):
            continue

        call = stmt.value
        if not isinstance(call, ast.Call):
            continue
        if not _is_column_call(call):
            continue

        col_name, col_type = _parse_column_args(call, field_name)
        columns.append({
            "name": col_name,
            "field": field_name,
            "type": col_type,
            "line": stmt.lineno,
        })

    return columns


def _is_column_call(call):
    """Check if a call node is Column(...)."""
    func = call.func
    if isinstance(func, ast.Name) and func.id == "Column":
        return True
    if isinstance(func, ast.Attribute) and func.attr == "Column":
        return True
    return False


def _parse_column_args(call, field_name):
    """Parse Column() arguments to extract column name and type.

    Handles:
      Column(Integer)                    -> name=field_name, type="Integer"
      Column("explicit_name", Integer)   -> name="explicit_name", type="Integer"
      Column(String(255))                -> name=field_name, type="String(255)"
      Column(Integer, nullable=False)    -> name=field_name, type="Integer"
    """
    col_name = field_name
    col_type = ""

    args = call.args
    if not args:
        return col_name, col_type

    idx = 0

    # Check if first arg is a string literal -> explicit column name.
    first = args[0]
    if isinstance(first, ast.Constant) and isinstance(first.value, str):
        col_name = first.value
        idx = 1

    # Find the type argument (Name or Call node).
    while idx < len(args):
        arg = args[idx]
        col_type = _type_from_node(arg)
        if col_type:
            break
        idx += 1

    return col_name, col_type


def _type_from_node(node):
    """Extract a type string from an AST node."""
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Call):
        func_name = ""
        if isinstance(node.func, ast.Name):
            func_name = node.func.id
        elif isinstance(node.func, ast.Attribute):
            func_name = node.func.attr
        if func_name:
            arg_strs = []
            for arg in node.args:
                if isinstance(arg, ast.Constant):
                    arg_strs.append(repr(arg.value))
                elif isinstance(arg, ast.Name):
                    arg_strs.append(arg.id)
            return f"{func_name}({', '.join(arg_strs)})"
    return ""


def main():
    """Entry point: extract models from files listed on the command line."""
    if len(sys.argv) < 2:
        json.dump([], sys.stdout)
        return

    all_results = []
    for filepath in sys.argv[1:]:
        try:
            all_results.extend(extract_models_from_file(filepath))
        except OSError as exc:
            print(f"reading python file {filepath}: {exc}", file=sys.stderr)
            sys.exit(2)
        except SyntaxError as exc:
            print(f"parsing python file {filepath}: {exc}", file=sys.stderr)
            sys.exit(2)

    json.dump(all_results, sys.stdout)


if __name__ == "__main__":
    main()
