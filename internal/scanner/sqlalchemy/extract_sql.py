# Copyright 2025 ValkDB
# SPDX-License-Identifier: Apache-2.0

"""Extract SQL strings from SQLAlchemy usage via Python AST analysis.

Usage: python3 extract_sql.py file1.py file2.py ...

Output: JSON array of objects with keys: file, line, column, end_line,
end_column, sql
"""

import ast
import json
import re
import sys


SYNTHETIC_PREFIX = "/* valk-guard:synthetic sqlalchemy-ast */ "
IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_.]*$")
JOIN_METHODS = {"join", "outerjoin", "leftouterjoin"}
FILTER_METHODS = {"filter", "filter_by", "where"}
FOR_UPDATE_METHODS = {"with_for_update"}


def _build_tablename_map(tree):
    """Build a mapping of Python class names to their __tablename__ values."""
    mapping = {}
    for node in ast.walk(tree):
        if not isinstance(node, ast.ClassDef):
            continue
        for item in node.body:
            if not isinstance(item, ast.Assign):
                continue
            for target in item.targets:
                if isinstance(target, ast.Name) and target.id == "__tablename__":
                    val = _get_string_value(item.value)
                    if val:
                        mapping[node.name] = val
    return mapping


def extract_sql_from_file(filepath, project_tablename_map=None):
    """Parse a Python file and extract raw and synthetic SQL strings."""
    with open(filepath, "r", encoding="utf-8", errors="replace") as f:
        source = f.read()

    tree = ast.parse(source, filename=filepath)

    results = []
    seen = set()
    handled_text_ids = set()
    parents = _build_parent_map(tree)

    _extract_raw_execute_text(tree, filepath, handled_text_ids, seen, results)
    tablename_map = dict(project_tablename_map or {})
    tablename_map.update(_build_tablename_map(tree))
    _extract_synthetic_chain_sql(tree, parents, filepath, seen, results, tablename_map)

    results.sort(key=lambda r: (r["line"], r.get("column", 1), r["sql"]))
    return results


def build_project_tablename_map(filepaths):
    """Build a best-effort __tablename__ map across all candidate Python files."""
    mapping = {}
    for filepath in filepaths:
        with open(filepath, "r", encoding="utf-8", errors="replace") as f:
            source = f.read()
        tree = ast.parse(source, filename=filepath)
        mapping.update(_build_tablename_map(tree))
    return mapping


def _extract_raw_execute_text(tree, filepath, handled_text_ids, seen, results):
    """Extract raw SQL from .execute() and text() literals."""
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue

        func = node.func
        if not (isinstance(func, ast.Attribute) and func.attr == "execute"):
            continue
        if not node.args:
            continue

        first_arg = node.args[0]

        if isinstance(first_arg, ast.Call):
            inner_func = first_arg.func
            if isinstance(inner_func, ast.Name) and inner_func.id == "text":
                handled_text_ids.add(id(first_arg))
                sql = _get_first_string_arg(first_arg)
                if sql is not None and sql.strip():
                    _append_unique(results, seen, filepath, node, sql.strip())
            continue

        sql = _get_string_value(first_arg)
        if sql is not None and sql.strip():
            _append_unique(results, seen, filepath, node, sql.strip())

    for node in ast.walk(tree):
        if isinstance(node, ast.Assign) and isinstance(node.value, ast.Call):
            _try_standalone_text(node.value, filepath, handled_text_ids, seen, results)
        elif isinstance(node, ast.Expr) and isinstance(node.value, ast.Call):
            _try_standalone_text(node.value, filepath, handled_text_ids, seen, results)


def _extract_synthetic_chain_sql(tree, parents, filepath, seen, results, tablename_map):
    """Extract synthetic SQL by analyzing SQLAlchemy method chains."""
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        if _is_chained_subcall(node, parents):
            continue

        synthetic = _synthetic_from_chain(node, tablename_map)
        if not synthetic:
            continue

        _append_unique(
            results,
            seen,
            filepath,
            node,
            SYNTHETIC_PREFIX + synthetic,
        )


def _synthetic_from_chain(node, tablename_map):
    """Return synthetic SQL for supported SQLAlchemy method chains."""
    chain = _flatten_call_chain(node)
    if not chain:
        return None

    root = chain[0]
    if root["kind"] == "method" and root["name"] == "query":
        return _synthesize_query_chain(chain, tablename_map)
    if root["kind"] == "func" and root["name"] == "select":
        return _synthesize_select_chain(chain, tablename_map)
    return None


def _synthesize_query_chain(chain, tablename_map):
    """Synthesize SQL from session.query(...) style chains."""
    root = chain[0]
    base_table, columns = _projection_from_query_root(root["args"], tablename_map)

    op_name, op_index = _operation_from_chain(chain)
    end = op_index if op_index is not None else len(chain)

    joins = []
    join_tables = []
    predicates = []
    has_filter = False
    has_limit = False
    has_for_update = False
    limit_val = "1"

    for link in chain[1:end]:
        method = link["name"].lower()
        if method in JOIN_METHODS:
            table = _table_name_from_expr(_first_arg(link["args"]), "synthetic_join", tablename_map)
            joins.append(_join_clause(method, table))
            join_tables.append(table)
            continue

        if method in FILTER_METHODS:
            has_filter = True
            conds = _predicates_from_filter_call(method, link["args"], link["keywords"], tablename_map)
            if not conds:
                conds = ["1=1"]
            predicates.extend(conds)
            continue

        if method == "limit":
            has_limit = True
            limit_val = _limit_from_args(link["args"])
            continue

        if method in FOR_UPDATE_METHODS:
            has_for_update = True

    if op_name == "update":
        return _build_update_sql(base_table, join_tables, has_filter, predicates)
    if op_name == "delete":
        return _build_delete_sql(base_table, join_tables, has_filter, predicates)

    return _build_select_sql(
        columns,
        base_table,
        joins,
        has_filter,
        predicates,
        has_limit,
        limit_val,
        has_for_update,
    )


def _synthesize_select_chain(chain, tablename_map):
    """Synthesize SQL from select(...) style chains."""
    root = chain[0]
    base_table, columns = _projection_from_select_root(root["args"], tablename_map)

    joins = []
    predicates = []
    has_filter = False
    has_limit = False
    has_for_update = False
    limit_val = "1"

    for link in chain[1:]:
        method = link["name"].lower()
        if method in JOIN_METHODS:
            table = _table_name_from_expr(_first_arg(link["args"]), "synthetic_join", tablename_map)
            joins.append(_join_clause(method, table))
            continue

        if method in FILTER_METHODS:
            has_filter = True
            conds = _predicates_from_filter_call(method, link["args"], link["keywords"], tablename_map)
            if not conds:
                conds = ["1=1"]
            predicates.extend(conds)
            continue

        if method == "limit":
            has_limit = True
            limit_val = _limit_from_args(link["args"])
            continue

        if method in FOR_UPDATE_METHODS:
            has_for_update = True

    return _build_select_sql(
        columns,
        base_table,
        joins,
        has_filter,
        predicates,
        has_limit,
        limit_val,
        has_for_update,
    )


def _projection_from_query_root(args, tablename_map):
    """Derive SELECT projection and base table from query(...) root args."""
    if not args:
        return "synthetic_model", ["*"]

    if len(args) == 1 and _is_model_expr(args[0]):
        return _table_name_from_expr(args[0], "synthetic_model", tablename_map), ["*"]

    columns = [_column_name_from_expr(arg, "", tablename_map) for arg in args]
    columns = [col for col in columns if col]
    if not columns:
        return "synthetic_model", ["*"]

    base_table = _table_from_columns(columns, "synthetic_model")
    return base_table, columns


def _projection_from_select_root(args, tablename_map):
    """Derive SELECT projection and base table from select(...) root args."""
    if not args:
        return "synthetic_model", ["*"]

    if len(args) == 1 and _is_model_expr(args[0]):
        return _table_name_from_expr(args[0], "synthetic_model", tablename_map), ["*"]

    columns = [_column_name_from_expr(arg, "", tablename_map) for arg in args]
    columns = [col for col in columns if col]
    if not columns:
        return "synthetic_model", ["*"]

    base_table = _table_from_columns(columns, "synthetic_model")
    return base_table, columns


def _operation_from_chain(chain):
    """Return terminal write operation (update/delete) and its index."""
    for idx, link in enumerate(chain[1:], start=1):
        method = link["name"].lower()
        if method in {"update", "delete"}:
            return method, idx
    return None, None


def _build_select_sql(columns, table, joins, has_filter, predicates, has_limit, limit_val, has_for_update):
    sql = f"SELECT {', '.join(columns)} FROM {table}"
    if joins:
        sql += " " + " ".join(joins)
    if has_filter and predicates:
        sql += " WHERE " + " AND ".join(predicates)
    if has_limit:
        sql += " LIMIT " + limit_val
    if has_for_update:
        sql += " FOR UPDATE"
    return sql


def _build_update_sql(table, join_tables, has_filter, predicates):
    sql = f"UPDATE {table} SET synthetic_col = 1"
    if join_tables:
        sql += " FROM " + ", ".join(join_tables)
    if has_filter and predicates:
        sql += " WHERE " + " AND ".join(predicates)
    return sql


def _build_delete_sql(table, join_tables, has_filter, predicates):
    sql = f"DELETE FROM {table}"
    if join_tables:
        sql += " USING " + ", ".join(join_tables)
    if has_filter and predicates:
        sql += " WHERE " + " AND ".join(predicates)
    return sql


def _join_clause(method, table):
    join_type = "JOIN"
    if method in {"outerjoin", "leftouterjoin"}:
        join_type = "LEFT JOIN"
    return f"{join_type} {table} ON 1=1"


def _predicates_from_filter_call(method, args, keywords, tablename_map):
    predicates = []

    if method == "filter_by":
        for kw in keywords:
            if not kw.arg:
                continue
            col = _safe_ident(kw.arg, "synthetic_col")
            predicates.append(f"{col} = {_sql_value(kw.value)}")

    for arg in args:
        pred = _predicate_from_expr(arg, tablename_map)
        if pred:
            predicates.append(pred)

    return predicates


def _predicate_from_expr(node, tablename_map):
    if isinstance(node, ast.Call):
        if isinstance(node.func, ast.Name) and node.func.id == "text":
            sql = _get_first_string_arg(node)
            if sql and sql.strip():
                return sql.strip()

        if isinstance(node.func, ast.Attribute):
            method = node.func.attr.lower()
            col = _column_name_from_expr(node.func.value, "synthetic_col", tablename_map)

            if method == "like":
                return f"{col} LIKE {_sql_value(_first_arg(node.args))}"
            if method == "ilike":
                return f"{col} ILIKE {_sql_value(_first_arg(node.args))}"
            if method == "contains":
                return f"{col} LIKE {_wrapped_like_value(_first_arg(node.args), '%', '%')}"
            if method == "startswith":
                return f"{col} LIKE {_wrapped_like_value(_first_arg(node.args), '', '%')}"
            if method == "endswith":
                return f"{col} LIKE {_wrapped_like_value(_first_arg(node.args), '%', '')}"

    if isinstance(node, ast.Compare):
        left = _column_name_from_expr(node.left, "synthetic_col", tablename_map)
        right = _sql_value(_first_arg(node.comparators))
        if not node.ops:
            return None
        op = _compare_op(node.ops[0])
        if op:
            return f"{left} {op} {right}"

    if isinstance(node, ast.BoolOp):
        pieces = [_predicate_from_expr(v, tablename_map) for v in node.values]
        pieces = [p for p in pieces if p]
        if not pieces:
            return None
        joiner = " AND " if isinstance(node.op, ast.And) else " OR "
        return "(" + joiner.join(pieces) + ")"

    return None


def _compare_op(op):
    if isinstance(op, ast.Eq):
        return "="
    if isinstance(op, ast.NotEq):
        return "<>"
    if isinstance(op, ast.Gt):
        return ">"
    if isinstance(op, ast.GtE):
        return ">="
    if isinstance(op, ast.Lt):
        return "<"
    if isinstance(op, ast.LtE):
        return "<="
    if isinstance(op, ast.In):
        return "IN"
    if isinstance(op, ast.NotIn):
        return "NOT IN"
    return None


def _wrapped_like_value(node, prefix, suffix):
    s = _string_literal(node)
    if s is None:
        return "'%synthetic%'"
    s = s.replace("'", "''")
    return "'" + prefix + s + suffix + "'"


def _table_name_from_expr(node, fallback, tablename_map=None):
    if isinstance(node, ast.Name):
        name = node.id
        if tablename_map and name in tablename_map:
            name = tablename_map[name]
        return _safe_ident(name, fallback)

    if isinstance(node, ast.Attribute):
        dotted = _attribute_to_dotted(node)
        tail = dotted.split(".")[-1] if dotted else ""
        if tablename_map and tail in tablename_map:
            tail = tablename_map[tail]
        return _safe_ident(tail, fallback)

    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return _safe_ident(node.value, fallback)

    if isinstance(node, ast.Call):
        # Handle wrappers like aliased(User)
        if node.args:
            return _table_name_from_expr(node.args[0], fallback, tablename_map)

    return fallback


def _column_name_from_expr(node, fallback, tablename_map=None):
    if isinstance(node, ast.Name):
        return _safe_ident(node.id, fallback)

    if isinstance(node, ast.Attribute):
        dotted = _attribute_to_dotted(node)
        if tablename_map and dotted:
            parts = dotted.split(".")
            if parts[0] in tablename_map:
                parts[0] = tablename_map[parts[0]]
            dotted = ".".join(parts)
        return _safe_ident(dotted, fallback)

    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return _safe_ident(node.value, fallback)

    if isinstance(node, ast.Call):
        # Handle wrappers such as func.lower(User.email)
        if isinstance(node.func, ast.Attribute):
            return _column_name_from_expr(node.func.value, fallback, tablename_map)
        if node.args:
            return _column_name_from_expr(node.args[0], fallback, tablename_map)

    return fallback


def _table_from_columns(columns, fallback):
    for col in columns:
        if "." in col:
            return _safe_ident(col.split(".")[0], fallback)
    return fallback


def _is_model_expr(node):
    name = ""
    if isinstance(node, ast.Name):
        name = node.id
    elif isinstance(node, ast.Attribute):
        dotted = _attribute_to_dotted(node)
        name = dotted.split(".")[-1] if dotted else ""
    if not name:
        return False
    return bool(name[:1].isupper())


def _limit_from_args(args):
    arg = _first_arg(args)
    if isinstance(arg, ast.Constant) and isinstance(arg.value, int):
        return str(arg.value)
    if isinstance(arg, ast.UnaryOp) and isinstance(arg.op, ast.USub):
        if isinstance(arg.operand, ast.Constant) and isinstance(arg.operand.value, int):
            return "-" + str(arg.operand.value)
    return "1"


def _sql_value(node):
    if node is None:
        return "NULL"

    if isinstance(node, ast.Constant):
        if isinstance(node.value, str):
            return "'" + node.value.replace("'", "''") + "'"
        if isinstance(node.value, bool):
            return "TRUE" if node.value else "FALSE"
        if node.value is None:
            return "NULL"
        if isinstance(node.value, (int, float)):
            return str(node.value)

    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
        if isinstance(node.operand, ast.Constant) and isinstance(node.operand.value, (int, float)):
            return "-" + str(node.operand.value)

    return "'synthetic_value'"


def _string_literal(node):
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    return None


def _attribute_to_dotted(node):
    parts = []
    current = node
    while isinstance(current, ast.Attribute):
        parts.append(current.attr)
        current = current.value
    if isinstance(current, ast.Name):
        parts.append(current.id)
    parts.reverse()
    return ".".join(parts)


def _flatten_call_chain(call):
    """Flatten method/function calls into a root->leaf chain."""
    reversed_chain = []
    current = call

    while isinstance(current, ast.Call):
        func = current.func
        if isinstance(func, ast.Attribute):
            reversed_chain.append({
                "kind": "method",
                "name": func.attr,
                "args": current.args,
                "keywords": current.keywords,
                "node": current,
            })
            if isinstance(func.value, ast.Call):
                current = func.value
                continue
            break

        if isinstance(func, ast.Name):
            reversed_chain.append({
                "kind": "func",
                "name": func.id,
                "args": current.args,
                "keywords": current.keywords,
                "node": current,
            })
            break

        return None

    if not reversed_chain:
        return None

    chain = list(reversed(reversed_chain))
    return chain


def _build_parent_map(tree):
    parents = {}
    for parent in ast.walk(tree):
        for child in ast.iter_child_nodes(parent):
            parents[child] = parent
    return parents


def _is_chained_subcall(call, parents):
    parent = parents.get(call)
    if not isinstance(parent, ast.Attribute) or parent.value is not call:
        return False

    grand = parents.get(parent)
    return isinstance(grand, ast.Call) and grand.func is parent


def _try_standalone_text(call, filepath, handled_ids, seen, results):
    """Append to results if call is a text("...") not already handled."""
    if id(call) in handled_ids:
        return
    func = call.func
    if not (isinstance(func, ast.Name) and func.id == "text"):
        return
    sql = _get_first_string_arg(call)
    if sql is not None and sql.strip():
        _append_unique(results, seen, filepath, call, sql.strip())


def _get_first_string_arg(call_node):
    """Return the string value of the first argument if it's a string literal."""
    if not call_node.args:
        return None
    return _get_string_value(call_node.args[0])


def _get_string_value(node):
    """Return the string value if the node is a string constant, else None."""
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    return None


def _append_unique(results, seen, filepath, node, sql):
    sql = sql.strip()
    if not sql:
        return
    line = getattr(node, "lineno", 1)
    end_line = getattr(node, "end_lineno", line)
    column = getattr(node, "col_offset", 0) + 1
    end_column = getattr(node, "end_col_offset", column) + 1
    key = (filepath, line, sql)
    if key in seen:
        return
    seen.add(key)
    results.append({
        "file": filepath,
        "line": line,
        "column": column,
        "end_line": end_line,
        "end_column": end_column,
        "sql": sql,
    })


def _safe_ident(raw, fallback):
    if not raw:
        return _quote_ident(fallback)
    raw = raw.strip().strip("\"`")
    if not raw:
        return _quote_ident(fallback)
    raw = raw.split()[0].rstrip(",")
    if raw == "*":
        return raw
    if IDENT_RE.match(raw):
        return _quote_ident(raw)
    return _quote_ident(fallback)


def _quote_ident(name):
    if not name:
        return ""
    if name == "*":
        return name
    parts = name.split(".")
    quoted = []
    for part in parts:
        part = part.strip().strip("\"`")
        if not part:
            continue
        quoted.append(f"\"{part}\"")
    if not quoted:
        return ""
    return ".".join(quoted)


def _first_arg(args):
    if not args:
        return None
    return args[0]


def main():
    """Entry point: extract SQL from files listed on the command line."""
    if len(sys.argv) < 2:
        json.dump([], sys.stdout)
        return

    all_results = []
    try:
        project_tablename_map = build_project_tablename_map(sys.argv[1:])
    except OSError as exc:
        print(f"reading python file while building project tablename map: {exc}", file=sys.stderr)
        sys.exit(2)
    except SyntaxError as exc:
        print(f"parsing python file while building project tablename map: {exc}", file=sys.stderr)
        sys.exit(2)

    for filepath in sys.argv[1:]:
        try:
            all_results.extend(extract_sql_from_file(filepath, project_tablename_map))
        except OSError as exc:
            print(f"reading python file {filepath}: {exc}", file=sys.stderr)
            sys.exit(2)
        except SyntaxError as exc:
            print(f"parsing python file {filepath}: {exc}", file=sys.stderr)
            sys.exit(2)

    json.dump(all_results, sys.stdout)


if __name__ == "__main__":
    main()
