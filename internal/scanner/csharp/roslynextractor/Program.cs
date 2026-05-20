// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

/// <summary>
/// Roslyn-based extractor for EF Core raw SQL calls and deterministic DbSet/LINQ query chains.
/// </summary>
internal static class Program
{
    private const string SyntheticPrefix = "/* valk-guard:synthetic csharp-efcore */ ";

    private static readonly HashSet<string> RawSqlMethods = new(StringComparer.Ordinal)
    {
        "ExecuteSqlRaw", "ExecuteSqlRawAsync", "ExecuteSqlInterpolated", "ExecuteSqlInterpolatedAsync",
        "FromSqlRaw", "FromSqlInterpolated", "SqlQueryRaw", "SqlQueryInterpolated",
    };

    private static readonly HashSet<string> TerminalMethods = new(StringComparer.Ordinal)
    {
        "ToList", "ToListAsync", "ToArray", "ToArrayAsync", "First", "FirstAsync",
        "FirstOrDefault", "FirstOrDefaultAsync", "Single", "SingleAsync", "SingleOrDefault",
        "SingleOrDefaultAsync", "Any", "AnyAsync", "All", "AllAsync", "Count", "CountAsync",
        "Sum", "SumAsync", "Min", "MinAsync", "Max", "MaxAsync", "Average", "AverageAsync",
        "ExecuteDelete", "ExecuteDeleteAsync", "ExecuteUpdate", "ExecuteUpdateAsync",
    };

    /// <summary>
    /// Parses all input files and prints JSON extraction results to stdout.
    /// </summary>
    public static int Main(string[] args)
    {
        var results = new List<SqlResult>();
        foreach (var file in args)
        {
            try
            {
                results.AddRange(ExtractFile(file));
            }
            catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
            {
                Console.Error.WriteLine($"{file}: {ex.Message}");
                return 2;
            }
        }

        Console.Write(JsonSerializer.Serialize(results));
        return 0;
    }

    /// <summary>
    /// Extracts raw and synthetic SQL statements from a single C# source file.
    /// </summary>
    private static IEnumerable<SqlResult> ExtractFile(string file)
    {
        var source = File.ReadAllText(file);
        var tree = CSharpSyntaxTree.ParseText(source, path: file);
        var root = tree.GetRoot();
        var tableMap = BuildTableMap(root);
        var results = new List<SqlResult>();

        foreach (var invocation in root.DescendantNodes().OfType<InvocationExpressionSyntax>())
        {
            if (TryExtractRawSql(file, invocation, out var rawResult))
            {
                results.Add(rawResult);
                continue;
            }

            if (TrySynthesizeQuerySql(file, invocation, tableMap, out var syntheticResult))
            {
                results.Add(syntheticResult);
            }
        }

        return results;
    }

    /// <summary>
    /// Extracts SQL from EF Core raw SQL APIs when the first argument resolves to a static string.
    /// </summary>
    private static bool TryExtractRawSql(string file, InvocationExpressionSyntax invocation, out SqlResult result)
    {
        result = default!;
        if (!TryGetInvokedMember(invocation, out var member))
        {
            return false;
        }

        var method = MethodName(member.Name);
        if (!RawSqlMethods.Contains(method) || !ReceiverLooksLikeEfCore(method, member.Expression, invocation))
        {
            return false;
        }
        if (invocation.ArgumentList.Arguments.Count == 0)
        {
            return false;
        }

        var locals = CollectLocalFragments(invocation);
        var placeholders = 1;
        var firstArg = invocation.ArgumentList.Arguments[0].Expression;
        if (!TryEvaluateString(firstArg, locals, ref placeholders, out var sql))
        {
            return false;
        }
        if (!method.Contains("Interpolated", StringComparison.Ordinal))
        {
            sql = NormalizeFormatPlaceholders(sql, invocation.ArgumentList.Arguments.Count - 1);
        }

        sql = NormalizeSql(sql);
        if (!LooksLikeSql(sql))
        {
            return false;
        }

        result = BuildResult(file, invocation, sql);
        return true;
    }

    /// <summary>
    /// Generates representative SQL for deterministic EF Core DbSet/LINQ query chains.
    /// </summary>
    private static bool TrySynthesizeQuerySql(
        string file,
        InvocationExpressionSyntax invocation,
        IReadOnlyDictionary<string, string> tableMap,
        out SqlResult result)
    {
        result = default!;
        var chain = UnwindChain(invocation);
        if (chain.Calls.Count == 0)
        {
            return false;
        }

        var terminal = chain.Calls[^1].Name;
        if (!TerminalMethods.Contains(terminal) || chain.Calls.Any(call => RawSqlMethods.Contains(call.Name)))
        {
            return false;
        }

        var table = ExtractBaseTable(chain, tableMap);
        if (table is null)
        {
            return false;
        }

        var sql = RenderQuery(chain, table, terminal, tableMap);
        if (string.IsNullOrWhiteSpace(sql))
        {
            return false;
        }

        result = BuildResult(file, invocation, SyntheticPrefix + sql);
        return true;
    }

    /// <summary>
    /// Builds an entity type to table-name map from attributes and simple Fluent API table configuration.
    /// </summary>
    private static Dictionary<string, string> BuildTableMap(SyntaxNode root)
    {
        var map = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var cls in root.DescendantNodes().OfType<ClassDeclarationSyntax>())
        {
            foreach (var list in cls.AttributeLists)
            {
                foreach (var attr in list.Attributes)
                {
                    var name = attr.Name.ToString();
                    if (!name.EndsWith("Table", StringComparison.Ordinal) && !name.EndsWith("TableAttribute", StringComparison.Ordinal))
                    {
                        continue;
                    }
                    var table = attr.ArgumentList?.Arguments.FirstOrDefault()?.Expression is LiteralExpressionSyntax lit
                        ? lit.Token.ValueText
                        : string.Empty;
                    var schema = attr.ArgumentList?.Arguments
                        .FirstOrDefault(arg => arg.NameEquals?.Name.Identifier.ValueText == "Schema")?.Expression is LiteralExpressionSyntax schemaLit
                            ? schemaLit.Token.ValueText
                            : string.Empty;
                    if (!string.IsNullOrWhiteSpace(table))
                    {
                        map[cls.Identifier.ValueText] = string.IsNullOrWhiteSpace(schema) ? SafeIdentifier(table, cls.Identifier.ValueText) : SafeIdentifier(schema + "." + table, cls.Identifier.ValueText);
                    }
                }
            }
        }

        foreach (var invocation in root.DescendantNodes().OfType<InvocationExpressionSyntax>())
        {
            if (!TryGetInvokedMember(invocation, out var member) || MethodName(member.Name) != "ToTable" || invocation.ArgumentList.Arguments.Count == 0)
            {
                continue;
            }
            if (invocation.ArgumentList.Arguments[0].Expression is not LiteralExpressionSyntax tableLiteral)
            {
                continue;
            }
            var table = SafeIdentifier(tableLiteral.Token.ValueText, "entity");
            if (member.Expression is InvocationExpressionSyntax entityCall && TryGetGenericTypeName(entityCall, out var entityType))
            {
                map[entityType] = table;
                continue;
            }
            if (member.Expression is IdentifierNameSyntax receiver)
            {
                var lambda = invocation.FirstAncestorOrSelf<SimpleLambdaExpressionSyntax>()
                    ?? invocation.FirstAncestorOrSelf<ParenthesizedLambdaExpressionSyntax>() as LambdaExpressionSyntax;
                if (lambda is not null && LambdaParameterName(lambda) == receiver.Identifier.ValueText)
                {
                    var entityInvocation = lambda.FirstAncestorOrSelf<InvocationExpressionSyntax>();
                    if (entityInvocation is not null && TryGetGenericTypeName(entityInvocation, out entityType))
                    {
                        map[entityType] = table;
                    }
                }
            }
        }

        return map;
    }

    /// <summary>
    /// Applies conservative receiver checks for raw EF Core SQL APIs.
    /// </summary>
    private static bool ReceiverLooksLikeEfCore(string method, ExpressionSyntax receiver, InvocationExpressionSyntax invocation)
    {
        if (method.StartsWith("FromSql", StringComparison.Ordinal))
        {
            return true;
        }

        receiver = UnwrapExpression(receiver);
        if (receiver is MemberAccessExpressionSyntax member && MethodName(member.Name) == "Database")
        {
            return true;
        }

        return receiver is IdentifierNameSyntax identifier && IsDatabaseFacadeName(invocation, identifier.Identifier.ValueText);
    }

    /// <summary>
    /// Reports whether the closest visible declaration/assignment before the call makes name a DatabaseFacade.
    /// </summary>
    private static bool IsDatabaseFacadeName(InvocationExpressionSyntax invocation, string name)
    {
        var method = invocation.FirstAncestorOrSelf<BaseMethodDeclarationSyntax>();
        if (method is not null)
        {
            foreach (var parameter in method.ParameterList.Parameters)
            {
                if (parameter.Identifier.ValueText == name && parameter.Type?.ToString().EndsWith("DatabaseFacade", StringComparison.Ordinal) == true)
                {
                    return true;
                }
            }
        }

        var scope = method as SyntaxNode ?? invocation.SyntaxTree.GetRoot();
        var invocationBlock = invocation.FirstAncestorOrSelf<BlockSyntax>();
        bool? closest = null;
        foreach (var node in scope.DescendantNodes().Where(node => node.Span.End < invocation.SpanStart).OrderBy(node => node.SpanStart))
        {
            if (node is LocalDeclarationStatementSyntax local)
            {
                if (invocationBlock is not null && local.FirstAncestorOrSelf<BlockSyntax>() != invocationBlock)
                {
                    continue;
                }
                foreach (var variable in local.Declaration.Variables)
                {
                    if (variable.Identifier.ValueText == name)
                    {
                        closest = local.Declaration.Type.ToString().EndsWith("DatabaseFacade", StringComparison.Ordinal)
                            || (variable.Initializer is not null && IsDatabaseProperty(variable.Initializer.Value));
                    }
                }
            }
            else if (node is AssignmentExpressionSyntax assignment && assignment.Left is IdentifierNameSyntax identifier && identifier.Identifier.ValueText == name)
            {
                if (IsConditionalFlow(assignment, method))
                {
                    return false;
                }
                closest = IsDatabaseProperty(assignment.Right);
            }
        }
        return closest == true;
    }


    /// <summary>
    /// Reports whether an assignment is inside conditional control flow before a call site.
    /// </summary>
    private static bool IsConditionalFlow(SyntaxNode node, BaseMethodDeclarationSyntax? method)
    {
        foreach (var ancestor in node.Ancestors())
        {
            if (method is not null && ancestor == method)
            {
                return false;
            }
            if (ancestor is IfStatementSyntax or SwitchStatementSyntax or ForStatementSyntax or ForEachStatementSyntax or WhileStatementSyntax or DoStatementSyntax)
            {
                return true;
            }
        }
        return false;
    }

    /// <summary>
    /// Reports whether expression syntactically references DbContext.Database.
    /// </summary>
    private static bool IsDatabaseProperty(ExpressionSyntax expression)
    {
        expression = UnwrapExpression(expression);
        return expression is MemberAccessExpressionSyntax member && MethodName(member.Name) == "Database";
    }

    /// <summary>
    /// Collects local string expression fragments visible before an invocation without flattening interpolation.
    /// </summary>
    private static Dictionary<string, ExpressionSyntax> CollectLocalFragments(InvocationExpressionSyntax invocation)
    {
        var locals = new Dictionary<string, ExpressionSyntax>(StringComparer.Ordinal);
        var scope = invocation.FirstAncestorOrSelf<BaseMethodDeclarationSyntax>() as SyntaxNode ?? invocation.SyntaxTree.GetRoot();
        var nodes = scope.DescendantNodes()
            .Where(node => node.Span.End < invocation.SpanStart)
            .Where(node => node is LocalDeclarationStatementSyntax or AssignmentExpressionSyntax)
            .OrderBy(node => node.SpanStart);

        foreach (var node in nodes)
        {
            if (node is LocalDeclarationStatementSyntax local)
            {
                foreach (var variable in local.Declaration.Variables)
                {
                    if (variable.Initializer is not null)
                    {
                        locals[variable.Identifier.ValueText] = variable.Initializer.Value;
                    }
                }
            }
            else if (node is AssignmentExpressionSyntax assignment && assignment.Left is IdentifierNameSyntax identifier)
            {
                locals[identifier.Identifier.ValueText] = assignment.Right;
            }
        }

        return locals;
    }

    /// <summary>
    /// Evaluates static SQL string expressions and allocates placeholders at the consuming call site.
    /// </summary>
    private static bool TryEvaluateString(ExpressionSyntax expression, IReadOnlyDictionary<string, ExpressionSyntax> locals, ref int nextPlaceholder, out string value)
    {
        return TryEvaluateString(expression, locals, new HashSet<string>(StringComparer.Ordinal), ref nextPlaceholder, out value);
    }

    /// <summary>
    /// Evaluates literals, concatenation, locals, and interpolation while preventing recursive local cycles.
    /// </summary>
    private static bool TryEvaluateString(
        ExpressionSyntax expression,
        IReadOnlyDictionary<string, ExpressionSyntax> locals,
        HashSet<string> resolving,
        ref int nextPlaceholder,
        out string value)
    {
        value = string.Empty;
        expression = UnwrapExpression(expression);
        if (expression is LiteralExpressionSyntax literal && literal.IsKind(SyntaxKind.StringLiteralExpression))
        {
            value = literal.Token.ValueText;
            return true;
        }
        if (expression is InterpolatedStringExpressionSyntax interpolated)
        {
            var parts = new List<string>();
            foreach (var content in interpolated.Contents)
            {
                if (content is InterpolatedStringTextSyntax text)
                {
                    parts.Add(text.TextToken.ValueText);
                }
                else if (content is InterpolationSyntax)
                {
                    parts.Add($"${nextPlaceholder++}");
                }
            }
            value = string.Concat(parts);
            return true;
        }
        if (expression is BinaryExpressionSyntax binary && binary.IsKind(SyntaxKind.AddExpression))
        {
            if (!TryEvaluateString(binary.Left, locals, resolving, ref nextPlaceholder, out var left))
            {
                return false;
            }
            if (!TryEvaluateString(binary.Right, locals, resolving, ref nextPlaceholder, out var right))
            {
                return false;
            }
            value = left + right;
            return true;
        }
        if (expression is IdentifierNameSyntax identifier && locals.TryGetValue(identifier.Identifier.ValueText, out var localExpression))
        {
            var localName = identifier.Identifier.ValueText;
            if (!resolving.Add(localName))
            {
                return false;
            }
            var ok = TryEvaluateString(localExpression, locals, resolving, ref nextPlaceholder, out value);
            resolving.Remove(localName);
            return ok;
        }
        return false;
    }

    /// <summary>
    /// Converts nested invocation/member syntax into an ordered query chain.
    /// </summary>
    private static QueryChain UnwindChain(InvocationExpressionSyntax invocation)
    {
        var calls = new List<QueryCall>();
        ExpressionSyntax current = invocation;
        while (current is InvocationExpressionSyntax currentInvocation && currentInvocation.Expression is MemberAccessExpressionSyntax member)
        {
            calls.Add(new QueryCall(MethodName(member.Name), currentInvocation));
            current = member.Expression;
        }
        calls.Reverse();
        return new QueryChain(current, calls);
    }

    /// <summary>
    /// Renders the collected query chain as parser-friendly synthetic SQL.
    /// </summary>
    private static string RenderQuery(QueryChain chain, string table, string terminal, IReadOnlyDictionary<string, string> tableMap)
    {
        var state = new QueryState(table);
        var binds = new PlaceholderAllocator();
        var afterGroupBy = false;

        foreach (var call in chain.Calls)
        {
            var name = call.Name;
            var args = call.Invocation.ArgumentList.Arguments.Select(arg => arg.Expression).ToArray();
            switch (name)
            {
                case "Distinct":
                    state.Distinct = true;
                    break;
                case "Select":
                    state.Columns = ExtractSelectColumns(args);
                    break;
                case "Take":
                    state.Limit = LiteralText(args.FirstOrDefault()) ?? "1";
                    break;
                case "Skip":
                    state.Offset = LiteralText(args.FirstOrDefault()) ?? "1";
                    break;
                case "OrderBy":
                case "ThenBy":
                case "OrderByDescending":
                case "ThenByDescending":
                    if (args.Length > 0 && ColumnFromLambda(args[0]) is { } orderCol)
                    {
                        state.OrderBy.Add(orderCol + (name.EndsWith("Descending", StringComparison.Ordinal) ? " DESC" : " ASC"));
                    }
                    break;
                case "GroupBy":
                    if (args.Length > 0 && ColumnFromLambda(args[0]) is { } groupCol)
                    {
                        state.GroupBy.Add(groupCol);
                        afterGroupBy = true;
                    }
                    break;
                case "Where":
                    if (args.Length > 0 && PredicateFromExpression(LambdaBody(args[0]), binds) is { } predicate)
                    {
                        if (afterGroupBy)
                        {
                            state.Having.Add(predicate);
                        }
                        else
                        {
                            state.Where.Add(predicate);
                        }
                    }
                    break;
                case "Join":
                    AddJoin(state, "JOIN", args, tableMap);
                    break;
                case "GroupJoin":
                    AddJoin(state, "LEFT JOIN", args, tableMap);
                    break;
                case "SelectMany":
                    if (!state.Joins.Any(join => join.Kind == "LEFT JOIN") && args.Length > 0)
                    {
                        var crossTable = TableFromLambda(args[0], tableMap) ?? "synthetic_join";
                        state.Joins.Add(new JoinClause("CROSS JOIN", crossTable, null));
                    }
                    break;
                case "Include":
                case "ThenInclude":
                    if (args.Length > 0 && ColumnFromLambda(args[0]) is { } includeTable)
                    {
                        state.Joins.Add(new JoinClause("LEFT JOIN", SafeIdentifier(includeTable, "joined"), null));
                    }
                    break;
                case "ForUpdate":
                case "LockingClause":
                    state.ForUpdate = true;
                    break;
            }
        }

        if (terminal.StartsWith("ExecuteDelete", StringComparison.Ordinal))
        {
            return BuildDeleteSql(state);
        }
        if (terminal.StartsWith("ExecuteUpdate", StringComparison.Ordinal))
        {
            state.Columns = ExtractUpdateSetColumns(chain.Calls[^1].Invocation);
            return BuildUpdateSql(state);
        }

        ApplyTerminalProjection(state, terminal, chain.Calls[^1].Invocation, binds);
        return BuildSelectSql(state);
    }

    /// <summary>
    /// Adds a join clause using table and key selector arguments when available.
    /// </summary>
    private static void AddJoin(QueryState state, string kind, IReadOnlyList<ExpressionSyntax> args, IReadOnlyDictionary<string, string> tableMap)
    {
        if (args.Count == 0)
        {
            return;
        }
        var table = TableFromExpression(args[0], tableMap) ?? "synthetic_join";
        string? on = null;
        if (args.Count >= 3)
        {
            var left = ColumnFromLambda(args[1]);
            var right = ColumnFromLambda(args[2]);
            if (!string.IsNullOrWhiteSpace(left) && !string.IsNullOrWhiteSpace(right))
            {
                on = left + " = " + right;
            }
        }
        state.Joins.Add(new JoinClause(kind, table, on));
    }

    /// <summary>
    /// Applies terminal aggregate or bounded-row projection behavior.
    /// </summary>
    private static void ApplyTerminalProjection(QueryState state, string terminal, InvocationExpressionSyntax invocation, PlaceholderAllocator placeholders)
    {
        var args = invocation.ArgumentList.Arguments.Select(arg => arg.Expression).ToArray();
        var normalized = terminal.EndsWith("Async", StringComparison.Ordinal) ? terminal[..^5] : terminal;
        switch (normalized)
        {
            case "Count":
                state.Columns = new List<string> { "COUNT(*)" };
                break;
            case "Any":
                state.Columns = new List<string> { "1" };
                state.Limit ??= "1";
                if (args.Length > 0 && PredicateFromExpression(LambdaBody(args[0]), placeholders) is { } anyPredicate)
                {
                    state.Where.Add(anyPredicate);
                }
                break;
            case "All":
                state.Columns = new List<string> { "1" };
                state.Limit ??= "1";
                if (args.Length > 0 && PredicateFromExpression(LambdaBody(args[0]), placeholders) is { } allPredicate)
                {
                    state.Where.Add(allPredicate);
                }
                break;
            case "Sum":
            case "Min":
            case "Max":
            case "Average":
                var aggregateColumn = args.Length > 0 ? ColumnFromLambda(args[0]) ?? "synthetic_col" : "synthetic_col";
                var fn = normalized == "Average" ? "AVG" : normalized.ToUpperInvariant();
                state.Columns = new List<string> { $"{fn}({aggregateColumn})" };
                break;
            case "First":
            case "FirstOrDefault":
            case "Single":
            case "SingleOrDefault":
                state.Limit ??= "1";
                break;
        }
    }

    /// <summary>
    /// Builds a synthetic SELECT statement with projections, joins, predicates, ordering, and bounds.
    /// </summary>
    private static string BuildSelectSql(QueryState state)
    {
        var columns = state.Columns.Count > 0 ? string.Join(", ", state.Columns) : "*";
        var distinct = state.Distinct ? "DISTINCT " : string.Empty;
        var sql = $"SELECT {distinct}{columns} FROM {state.Table}";
        if (state.Joins.Count > 0)
        {
            sql += " " + string.Join(" ", state.Joins.Select(join => join.ToSql()));
        }
        if (state.Where.Count > 0)
        {
            sql += " WHERE " + string.Join(" AND ", state.Where);
        }
        if (state.GroupBy.Count > 0)
        {
            sql += " GROUP BY " + string.Join(", ", state.GroupBy);
        }
        if (state.Having.Count > 0)
        {
            sql += " HAVING " + string.Join(" AND ", state.Having);
        }
        if (state.OrderBy.Count > 0)
        {
            sql += " ORDER BY " + string.Join(", ", state.OrderBy);
        }
        if (state.Limit is not null)
        {
            sql += " LIMIT " + state.Limit;
        }
        if (state.Offset is not null)
        {
            sql += " OFFSET " + state.Offset;
        }
        if (state.ForUpdate)
        {
            sql += " FOR UPDATE";
        }
        return sql;
    }

    /// <summary>
    /// Builds a synthetic DELETE statement, preserving explicit query joins and predicates.
    /// </summary>
    private static string BuildDeleteSql(QueryState state)
    {
        var sql = $"DELETE FROM {state.Table}";
        var joinedTables = state.Joins.Where(join => join.Kind != "LEFT JOIN" || join.On is not null).Select(join => join.Table).ToArray();
        if (joinedTables.Length > 0)
        {
            sql += " USING " + string.Join(", ", joinedTables);
        }
        if (state.Where.Count > 0)
        {
            sql += " WHERE " + string.Join(" AND ", state.Where);
        }
        return sql;
    }

    /// <summary>
    /// Builds a synthetic UPDATE statement with a parser-safe SET clause.
    /// </summary>
    private static string BuildUpdateSql(QueryState state)
    {
        var setColumn = state.Columns.FirstOrDefault(column => column != "*") ?? "synthetic_col";
        var sql = $"UPDATE {state.Table} SET {setColumn} = $1";
        var joinedTables = state.Joins.Where(join => join.Kind != "LEFT JOIN" || join.On is not null).Select(join => join.Table).ToArray();
        if (joinedTables.Length > 0)
        {
            sql += " FROM " + string.Join(", ", joinedTables);
        }
        if (state.Where.Count > 0)
        {
            sql += " WHERE " + string.Join(" AND ", state.Where);
        }
        return sql;
    }

    /// <summary>
    /// Extracts columns targeted by ExecuteUpdate SetProperty calls when visible in the AST.
    /// </summary>
    private static List<string> ExtractUpdateSetColumns(InvocationExpressionSyntax invocation)
    {
        var columns = new List<string>();
        foreach (var setProperty in invocation.DescendantNodes().OfType<InvocationExpressionSyntax>())
        {
            if (!TryGetInvokedMember(setProperty, out var member) || MethodName(member.Name) != "SetProperty")
            {
                continue;
            }
            if (setProperty.ArgumentList.Arguments.Count == 0)
            {
                continue;
            }
            var column = ColumnFromLambda(setProperty.ArgumentList.Arguments[0].Expression);
            if (!string.IsNullOrWhiteSpace(column))
            {
                columns.Add(column);
            }
        }
        return columns.Count == 0 ? new List<string> { "synthetic_col" } : columns.Distinct(StringComparer.Ordinal).ToList();
    }

    /// <summary>
    /// Extracts the base table from DbSet member access or DbContext.Set&lt;T&gt;().
    /// </summary>
    private static string? ExtractBaseTable(QueryChain chain, IReadOnlyDictionary<string, string> tableMap)
    {
        foreach (var call in chain.Calls)
        {
            if (call.Name == "Set" && TryGetGenericTypeName(call.Invocation, out var setType))
            {
                return ResolveTable(setType, tableMap);
            }
        }
        return TableFromExpression(chain.BaseExpression, tableMap);
    }

    /// <summary>
    /// Extracts projection columns from Select(lambda) calls when statically recognizable.
    /// </summary>
    private static List<string> ExtractSelectColumns(IReadOnlyList<ExpressionSyntax> args)
    {
        if (args.Count == 0)
        {
            return new List<string> { "*" };
        }
        var body = LambdaBody(args[0]) ?? args[0];
        if (body is AnonymousObjectCreationExpressionSyntax anonymous)
        {
            var columns = anonymous.Initializers.Select(initializer => ColumnFromExpression(initializer.Expression)).WhereNotNull().Distinct(StringComparer.Ordinal).ToList();
            return columns.Count == 0 ? new List<string> { "synthetic_col" } : columns;
        }
        return ColumnFromExpression(body) is { } singleColumn ? new List<string> { singleColumn } : new List<string> { "synthetic_col" };
    }

    /// <summary>
    /// Converts a recognizable C# predicate expression into a SQL predicate.
    /// </summary>
    private static string? PredicateFromExpression(ExpressionSyntax? expression, PlaceholderAllocator placeholders)
    {
        if (expression is null)
        {
            return null;
        }
        expression = UnwrapExpression(expression);
        if (expression is PrefixUnaryExpressionSyntax prefix && prefix.IsKind(SyntaxKind.LogicalNotExpression))
        {
            var inner = PredicateFromExpression(prefix.Operand, placeholders);
            return inner is null ? null : $"NOT ({inner})";
        }
        if (expression is BinaryExpressionSyntax binary)
        {
            if (binary.IsKind(SyntaxKind.LogicalAndExpression) || binary.IsKind(SyntaxKind.LogicalOrExpression))
            {
                var left = PredicateFromExpression(binary.Left, placeholders);
                var right = PredicateFromExpression(binary.Right, placeholders);
                if (left is null || right is null)
                {
                    return left ?? right;
                }
                return $"({left}) {(binary.IsKind(SyntaxKind.LogicalAndExpression) ? "AND" : "OR")} ({right})";
            }
            var column = ColumnFromExpression(binary.Left);
            var value = SqlValue(binary.Right, placeholders);
            var op = SqlOperator(binary.Kind());
            if (column is not null && value is not null && op is not null)
            {
                return $"{column} {op} {value}";
            }
        }
        if (expression is InvocationExpressionSyntax invocation)
        {
            return PredicateFromInvocation(invocation, placeholders);
        }
        return ColumnFromExpression(expression) is { } boolColumn ? $"{boolColumn} = TRUE" : null;
    }

    /// <summary>
    /// Converts invocation predicates such as Like, Contains, and collection Contains into SQL predicates.
    /// </summary>
    private static string? PredicateFromInvocation(InvocationExpressionSyntax invocation, PlaceholderAllocator placeholders)
    {
        if (!TryGetInvokedMember(invocation, out var member))
        {
            return null;
        }
        var method = MethodName(member.Name);
        if (method is "Like" or "ILike" && invocation.ArgumentList.Arguments.Count >= 2 && IsEfFunctionsReceiver(member.Expression))
        {
            var column = ColumnFromExpression(invocation.ArgumentList.Arguments[0].Expression);
            var pattern = StringPattern(invocation.ArgumentList.Arguments[1].Expression, PatternKind.Exact, placeholders);
            var op = method == "ILike" ? "ILIKE" : "LIKE";
            return column is null || pattern is null ? null : $"{column} {op} {pattern}";
        }
        if (method is "Contains" or "StartsWith" or "EndsWith" && invocation.ArgumentList.Arguments.Count > 0)
        {
            var argColumn = ColumnFromExpression(invocation.ArgumentList.Arguments[0].Expression);
            if (method == "Contains" && argColumn is not null && member.Expression is not MemberAccessExpressionSyntax)
            {
                return $"{argColumn} IN ({SyntheticBindList(member.Expression, placeholders)})";
            }
            var receiverColumn = ColumnFromExpression(member.Expression);
            var kind = method switch
            {
                "Contains" => PatternKind.Contains,
                "StartsWith" => PatternKind.StartsWith,
                "EndsWith" => PatternKind.EndsWith,
                _ => PatternKind.Exact,
            };
            var pattern = StringPattern(invocation.ArgumentList.Arguments[0].Expression, kind, placeholders);
            return receiverColumn is null || pattern is null ? null : $"{receiverColumn} LIKE {pattern}";
        }
        return null;
    }

    /// <summary>
    /// Produces a bind placeholder list, using static collection arity when available.
    /// </summary>
    private static string SyntheticBindList(ExpressionSyntax collection, PlaceholderAllocator placeholders)
    {
        var arity = 3;
        collection = UnwrapExpression(collection);
        if (collection is ImplicitArrayCreationExpressionSyntax implicitArray)
        {
            arity = Math.Max(1, implicitArray.Initializer.Expressions.Count);
        }
        else if (collection is ArrayCreationExpressionSyntax array && array.Initializer is not null)
        {
            arity = Math.Max(1, array.Initializer.Expressions.Count);
        }
        else if (collection is ObjectCreationExpressionSyntax obj && obj.Initializer is not null)
        {
            arity = Math.Max(1, obj.Initializer.Expressions.Count);
        }
        return string.Join(", ", Enumerable.Range(0, arity).Select(_ => placeholders.Next()));
    }

    /// <summary>
    /// Converts a static expression into a SQL literal or placeholder.
    /// </summary>
    private static string? SqlValue(ExpressionSyntax expression, PlaceholderAllocator placeholders)
    {
        expression = UnwrapExpression(expression);
        if (expression is LiteralExpressionSyntax literal)
        {
            if (literal.IsKind(SyntaxKind.StringLiteralExpression))
            {
                return QuoteString(literal.Token.ValueText);
            }
            if (literal.IsKind(SyntaxKind.TrueLiteralExpression))
            {
                return "TRUE";
            }
            if (literal.IsKind(SyntaxKind.FalseLiteralExpression))
            {
                return "FALSE";
            }
            if (literal.IsKind(SyntaxKind.NullLiteralExpression))
            {
                return "NULL";
            }
            return literal.Token.ValueText;
        }
        return expression is IdentifierNameSyntax or MemberAccessExpressionSyntax or InvocationExpressionSyntax ? placeholders.Next() : null;
    }

    /// <summary>
    /// Converts a string expression into a quoted SQL LIKE pattern.
    /// </summary>
    private static string? StringPattern(ExpressionSyntax expression, PatternKind kind, PlaceholderAllocator placeholders)
    {
        expression = UnwrapExpression(expression);
        if (expression is LiteralExpressionSyntax literal && literal.IsKind(SyntaxKind.StringLiteralExpression))
        {
            var value = literal.Token.ValueText;
            value = kind switch
            {
                PatternKind.Contains => "%" + value + "%",
                PatternKind.StartsWith => value + "%",
                PatternKind.EndsWith => "%" + value,
                _ => value,
            };
            return QuoteString(value);
        }
        return expression is IdentifierNameSyntax or MemberAccessExpressionSyntax or InvocationExpressionSyntax ? placeholders.Next() : null;
    }

    /// <summary>
    /// Extracts a SQL column-like identifier from member access or identifier syntax.
    /// </summary>
    private static string? ColumnFromExpression(ExpressionSyntax expression)
    {
        expression = UnwrapExpression(expression);
        if (expression is MemberAccessExpressionSyntax member)
        {
            return SafeIdentifier(MethodName(member.Name), "col");
        }
        if (expression is IdentifierNameSyntax identifier)
        {
            return SafeIdentifier(identifier.Identifier.ValueText, "col");
        }
        return null;
    }

    /// <summary>
    /// Extracts a column identifier from a lambda expression body.
    /// </summary>
    private static string? ColumnFromLambda(ExpressionSyntax expression)
    {
        return LambdaBody(expression) is { } body ? ColumnFromExpression(body) : ColumnFromExpression(expression);
    }

    /// <summary>
    /// Extracts a table identifier from a lambda expression body.
    /// </summary>
    private static string? TableFromLambda(ExpressionSyntax expression, IReadOnlyDictionary<string, string> tableMap)
    {
        var body = LambdaBody(expression);
        return body is null ? null : TableFromExpression(body, tableMap);
    }

    /// <summary>
    /// Converts a DbSet-like expression into a SQL table identifier.
    /// </summary>
    private static string? TableFromExpression(ExpressionSyntax expression, IReadOnlyDictionary<string, string> tableMap)
    {
        expression = UnwrapExpression(expression);
        if (expression is MemberAccessExpressionSyntax member)
        {
            return ResolveTable(MethodName(member.Name), tableMap);
        }
        if (expression is IdentifierNameSyntax identifier)
        {
            return ResolveTable(identifier.Identifier.ValueText, tableMap);
        }
        if (expression is InvocationExpressionSyntax invocation && TryGetGenericTypeName(invocation, out var genericType))
        {
            return ResolveTable(genericType, tableMap);
        }
        return null;
    }

    /// <summary>
    /// Resolves an entity or DbSet identifier to its configured table name.
    /// </summary>
    private static string ResolveTable(string value, IReadOnlyDictionary<string, string> tableMap)
    {
        var rightmost = RightmostIdentifier(value);
        if (tableMap.TryGetValue(rightmost, out var mapped))
        {
            return mapped;
        }
        if (rightmost.EndsWith("s", StringComparison.Ordinal) && tableMap.TryGetValue(rightmost[..^1], out mapped))
        {
            return mapped;
        }
        return SafeIdentifier(rightmost, "entity");
    }

    /// <summary>
    /// Returns the expression body of a lambda expression.
    /// </summary>
    private static ExpressionSyntax? LambdaBody(ExpressionSyntax expression)
    {
        expression = UnwrapExpression(expression);
        return expression switch
        {
            SimpleLambdaExpressionSyntax simple when simple.Body is ExpressionSyntax body => body,
            ParenthesizedLambdaExpressionSyntax parenthesized when parenthesized.Body is ExpressionSyntax body => body,
            _ => null,
        };
    }

    /// <summary>
    /// Returns the first parameter name of a lambda expression.
    /// </summary>
    private static string? LambdaParameterName(LambdaExpressionSyntax lambda)
    {
        return lambda switch
        {
            SimpleLambdaExpressionSyntax simple => simple.Parameter.Identifier.ValueText,
            ParenthesizedLambdaExpressionSyntax parenthesized => parenthesized.ParameterList.Parameters.FirstOrDefault()?.Identifier.ValueText,
            _ => null,
        };
    }

    /// <summary>
    /// Extracts a generic type name from an invocation such as Set&lt;User&gt;().
    /// </summary>
    private static bool TryGetGenericTypeName(InvocationExpressionSyntax invocation, out string typeName)
    {
        typeName = string.Empty;
        if (invocation.Expression is not MemberAccessExpressionSyntax member || member.Name is not GenericNameSyntax generic || generic.TypeArgumentList.Arguments.Count == 0)
        {
            return false;
        }
        typeName = TypeIdentifier(generic.TypeArgumentList.Arguments[0]);
        return true;
    }

    /// <summary>
    /// Returns the rightmost type identifier from a possibly qualified generic type syntax.
    /// </summary>
    private static string TypeIdentifier(TypeSyntax type)
    {
        type = type is NullableTypeSyntax nullable ? nullable.ElementType : type;
        return type switch
        {
            IdentifierNameSyntax identifier => identifier.Identifier.ValueText,
            GenericNameSyntax generic => generic.Identifier.ValueText,
            QualifiedNameSyntax qualified => TypeIdentifier(qualified.Right),
            AliasQualifiedNameSyntax aliasQualified => TypeIdentifier(aliasQualified.Name),
            _ => RightmostIdentifier(type.ToString()),
        };
    }

    /// <summary>
    /// Extracts the rightmost identifier from fallback type text.
    /// </summary>
    private static string RightmostIdentifier(string text)
    {
        var candidate = text.Split('.').LastOrDefault() ?? text;
        var genericStart = candidate.IndexOf('<', StringComparison.Ordinal);
        return genericStart >= 0 ? candidate[..genericStart] : candidate;
    }

    /// <summary>
    /// Returns the invoked member access for an invocation expression.
    /// </summary>
    private static bool TryGetInvokedMember(InvocationExpressionSyntax invocation, out MemberAccessExpressionSyntax member)
    {
        member = default!;
        if (invocation.Expression is not MemberAccessExpressionSyntax candidate)
        {
            return false;
        }
        member = candidate;
        return true;
    }

    /// <summary>
    /// Returns true when the expression is a reference to <c>EF.Functions</c>.
    /// Recognizes the canonical Microsoft.EntityFrameworkCore receiver shape used by
    /// <c>EF.Functions.Like</c> / <c>EF.Functions.ILike</c> and similar extension entry points.
    /// </summary>
    private static bool IsEfFunctionsReceiver(ExpressionSyntax? expression)
    {
        if (expression is not MemberAccessExpressionSyntax member)
        {
            return false;
        }
        if (MethodName(member.Name) != "Functions")
        {
            return false;
        }
        return member.Expression is IdentifierNameSyntax id && id.Identifier.ValueText == "EF";
    }

    /// <summary>
    /// Returns a method identifier from an identifier or generic method name syntax node.
    /// </summary>
    private static string MethodName(SimpleNameSyntax name)
    {
        return name switch
        {
            GenericNameSyntax generic => generic.Identifier.ValueText,
            IdentifierNameSyntax identifier => identifier.Identifier.ValueText,
            _ => name.ToString(),
        };
    }

    /// <summary>
    /// Removes syntactic wrappers that do not affect expression meaning for extraction.
    /// </summary>
    private static ExpressionSyntax UnwrapExpression(ExpressionSyntax expression)
    {
        while (expression is ParenthesizedExpressionSyntax parenthesized)
        {
            expression = parenthesized.Expression;
        }
        return expression;
    }

    /// <summary>
    /// Converts ExecuteSqlRaw-style format placeholders to PostgreSQL placeholders.
    /// </summary>
    private static string NormalizeFormatPlaceholders(string sql, int valueArgumentCount)
    {
        for (var i = 0; i < valueArgumentCount; i++)
        {
            sql = sql.Replace("{" + i + "}", "$" + (i + 1), StringComparison.Ordinal);
        }
        return sql;
    }

    /// <summary>
    /// Maps C# binary expression kinds to SQL operators.
    /// </summary>
    private static string? SqlOperator(SyntaxKind kind)
    {
        return kind switch
        {
            SyntaxKind.EqualsExpression => "=",
            SyntaxKind.NotEqualsExpression => "<>",
            SyntaxKind.GreaterThanExpression => ">",
            SyntaxKind.GreaterThanOrEqualExpression => ">=",
            SyntaxKind.LessThanExpression => "<",
            SyntaxKind.LessThanOrEqualExpression => "<=",
            _ => null,
        };
    }

    /// <summary>
    /// Returns a literal expression value that is safe for LIMIT/OFFSET rendering.
    /// </summary>
    private static string? LiteralText(ExpressionSyntax? expression)
    {
        expression = expression is null ? null : UnwrapExpression(expression);
        return expression is LiteralExpressionSyntax literal && literal.Token.Value is not null ? literal.Token.ValueText : null;
    }

    /// <summary>
    /// Escapes a string for use as a SQL single-quoted literal.
    /// </summary>
    private static string QuoteString(string value)
    {
        return "'" + value.Replace("'", "''", StringComparison.Ordinal) + "'";
    }

    /// <summary>
    /// Normalizes whitespace so generated SQL is parser-friendly and stable in tests.
    /// </summary>
    private static string NormalizeSql(string sql)
    {
        return string.Join(" ", sql.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries)).Trim();
    }

    /// <summary>
    /// Reports whether a string starts like SQL or a synthetic SQL comment.
    /// </summary>
    private static bool LooksLikeSql(string sql)
    {
        sql = sql.TrimStart();
        if (sql.StartsWith("/*", StringComparison.Ordinal) || sql.StartsWith("--", StringComparison.Ordinal))
        {
            return true;
        }
        var firstSpace = sql.IndexOfAny(new[] { ' ', '\t', '\r', '\n' });
        var first = firstSpace < 0 ? sql : sql[..firstSpace];
        return first is "SELECT" or "INSERT" or "UPDATE" or "DELETE" or "CREATE" or "DROP" or "ALTER" or "TRUNCATE" or "WITH" or "GRANT" or "REVOKE" or "BEGIN" or "COMMIT" or "ROLLBACK" or "SET" or "COPY" or "VACUUM" or "ANALYZE" or "EXPLAIN" or "MERGE";
    }

    /// <summary>
    /// Sanitizes an extracted identifier enough for synthetic SQL parsing.
    /// </summary>
    private static string SafeIdentifier(string value, string fallback)
    {
        var trimmed = value.Trim();
        if (trimmed.Length == 0)
        {
            return fallback;
        }
        var chars = trimmed.Where(ch => char.IsLetterOrDigit(ch) || ch == '_' || ch == '.').ToArray();
        return chars.Length == 0 ? fallback : new string(chars);
    }

    /// <summary>
    /// Builds a JSON result with one-based source coordinates from a syntax node.
    /// </summary>
    private static SqlResult BuildResult(string file, SyntaxNode node, string sql)
    {
        var span = node.GetLocation().GetLineSpan();
        return new SqlResult
        {
            File = file,
            Line = span.StartLinePosition.Line + 1,
            Column = span.StartLinePosition.Character + 1,
            EndLine = span.EndLinePosition.Line + 1,
            EndColumn = span.EndLinePosition.Character + 1,
            Sql = sql,
        };
    }

    /// <summary>
    /// Describes how a SQL LIKE pattern should be wrapped.
    /// </summary>
    private enum PatternKind
    {
        Exact,
        Contains,
        StartsWith,
        EndsWith,
    }

    /// <summary>
    /// Allocates stable PostgreSQL-style placeholders for one synthetic SQL statement.
    /// </summary>
    private sealed class PlaceholderAllocator
    {
        private int next = 1;

        /// <summary>
        /// Returns the next numbered bind placeholder.
        /// </summary>
        public string Next()
        {
            return "$" + next++;
        }
    }

    /// <summary>
    /// Mutable representation of one synthetic SQL statement while a chain is analyzed.
    /// </summary>
    private sealed class QueryState
    {
        /// <summary>
        /// Creates a state object for a base table.
        /// </summary>
        public QueryState(string table)
        {
            Table = table;
        }

        public string Table { get; }
        public bool Distinct { get; set; }
        public bool ForUpdate { get; set; }
        public string? Limit { get; set; }
        public string? Offset { get; set; }
        public List<string> Columns { get; set; } = new();
        public List<JoinClause> Joins { get; } = new();
        public List<string> Where { get; } = new();
        public List<string> GroupBy { get; } = new();
        public List<string> Having { get; } = new();
        public List<string> OrderBy { get; } = new();
    }

    /// <summary>
    /// Holds one method call in a fluent query chain.
    /// </summary>
    private sealed record QueryCall(string Name, InvocationExpressionSyntax Invocation);

    /// <summary>
    /// Holds the base expression and ordered calls for a fluent query chain.
    /// </summary>
    private sealed record QueryChain(ExpressionSyntax BaseExpression, List<QueryCall> Calls);

    /// <summary>
    /// Holds one synthetic SQL join target.
    /// </summary>
    private sealed record JoinClause(string Kind, string Table, string? On)
    {
        /// <summary>
        /// Converts the join target to parser-friendly SQL.
        /// </summary>
        public string ToSql() => Kind == "CROSS JOIN" ? $"{Kind} {Table}" : $"{Kind} {Table} ON {On ?? "1=1"}";
    }
}

/// <summary>
/// LINQ helpers that keep nullable filtering terse without hiding scanner logic.
/// </summary>
internal static class EnumerableExtensions
{
    /// <summary>
    /// Filters null strings from a sequence.
    /// </summary>
    public static IEnumerable<string> WhereNotNull(this IEnumerable<string?> values)
    {
        foreach (var value in values)
        {
            if (value is not null)
            {
                yield return value;
            }
        }
    }
}

/// <summary>
/// JSON DTO for a single extracted SQL statement.
/// </summary>
internal sealed class SqlResult
{
    [JsonPropertyName("file")]
    public string File { get; init; } = string.Empty;

    [JsonPropertyName("line")]
    public int Line { get; init; }

    [JsonPropertyName("column")]
    public int Column { get; init; }

    [JsonPropertyName("end_line")]
    public int EndLine { get; init; }

    [JsonPropertyName("end_column")]
    public int EndColumn { get; init; }

    [JsonPropertyName("sql")]
    public string Sql { get; init; } = string.Empty;
}
