// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

using System.Diagnostics;
using System.Text.Json;
using System.Text.Json.Serialization;
using Xunit;

namespace RoslynExtractor.Tests;

public sealed class ExtractorGoldenTests
{
    [Fact]
    public async Task RawSqlExtractionPreservesJsonEscapingAndLocation()
    {
        var source = """
            class Repo {
                void Run(MyDbContext db) {
                    db.Database.ExecuteSqlRaw("SELECT \"name\", '\\\\path' FROM users WHERE note = 'line\\nnext'");
                }
            }
            """;

        var results = await ExtractAsync(source);

        var result = Assert.Single(results);
        Assert.Equal("SELECT \"name\", '\\\\path' FROM users WHERE note = 'line\\nnext'", result.Sql);
        Assert.Equal(3, result.Line);
        Assert.True(result.Column > 0);
        Assert.True(result.EndColumn > result.Column);
    }

    [Fact]
    public async Task MultiLineRawStringReportsEndLine()
    {
        var source = """"
            class Repo {
                void Run(MyDbContext db) {
                    db.Database.ExecuteSqlRaw("""
                        SELECT id
                        FROM users
                        WHERE active = true
                        LIMIT 10
                        """);
                }
            }
            """";

        var results = await ExtractAsync(source);

        var result = Assert.Single(results);
        Assert.Contains("SELECT id", result.Sql, StringComparison.Ordinal);
        Assert.Contains("LIMIT 10", result.Sql, StringComparison.Ordinal);
        Assert.True(result.EndLine > result.Line, $"expected multi-line span, got {result.Line}:{result.Column}-{result.EndLine}:{result.EndColumn}");
    }

    [Fact]
    public async Task TerminalMethodsProduceSyntheticSql()
    {
        var source = """
            class Repo {
                void Run(MyDbContext db) {
                    db.Users.Take(1).ToList();
                    db.Users.Take(1).ToArray();
                    db.Users.Where(u => u.Id == 1).First();
                    db.Users.Where(u => u.Id == 1).FirstOrDefault();
                    db.Users.Where(u => u.Id == 1).Single();
                    db.Users.Where(u => u.Id == 1).SingleOrDefault();
                    db.Users.ExecuteDelete();
                    db.Users.ExecuteUpdate(s => s.SetProperty(u => u.Active, false));
                }
            }
            """;

        var sql = (await ExtractAsync(source)).Select(result => result.Sql).ToArray();

        Assert.Contains(sql, text => text.Contains("SELECT * FROM Users LIMIT 1", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("DELETE FROM Users", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("UPDATE Users SET Active = $1", StringComparison.Ordinal));
        Assert.True(sql.Length >= 8, $"expected one statement per terminal call, got {sql.Length}: {string.Join(" | ", sql)}");
    }

    [Fact]
    public async Task AggregatesProduceSyntheticSql()
    {
        var source = """
            class Repo {
                void Run(MyDbContext db, int minId) {
                    db.Users.Count();
                    db.Users.Any();
                    db.Users.All(u => u.Id > minId);
                    db.Orders.Sum(o => o.Total);
                    db.Orders.Min(o => o.Total);
                    db.Orders.Max(o => o.Total);
                    db.Orders.Average(o => o.Total);
                }
            }
            """;

        var sql = (await ExtractAsync(source)).Select(result => result.Sql).ToArray();

        Assert.Contains(sql, text => text.Contains("SELECT COUNT(*) FROM Users", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("SELECT 1 FROM Users LIMIT 1", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("WHERE Id > $1", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("SELECT SUM(Total) FROM Orders", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("SELECT MIN(Total) FROM Orders", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("SELECT MAX(Total) FROM Orders", StringComparison.Ordinal));
        Assert.Contains(sql, text => text.Contains("SELECT AVG(Total) FROM Orders", StringComparison.Ordinal));
    }

    private static async Task<IReadOnlyList<ExtractorResult>> ExtractAsync(string source)
    {
        var tempDir = Directory.CreateTempSubdirectory("valk-roslyn-test-");
        try
        {
            var sourceFile = Path.Combine(tempDir.FullName, "input.cs");
            await File.WriteAllTextAsync(sourceFile, source);

            var project = FindExtractorProject();
            var dotnet = Environment.GetEnvironmentVariable("DOTNET_HOST_PATH") ?? "dotnet";
            var startInfo = new ProcessStartInfo(dotnet, $"run --project \"{project}\" -- \"{sourceFile}\"")
            {
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                UseShellExecute = false,
            };
            startInfo.Environment["DOTNET_CLI_TELEMETRY_OPTOUT"] = "1";
            startInfo.Environment["DOTNET_NOLOGO"] = "1";
            startInfo.Environment["DOTNET_SKIP_FIRST_TIME_EXPERIENCE"] = "1";

            using var process = Process.Start(startInfo) ?? throw new InvalidOperationException("failed to start dotnet");
            var stdout = await process.StandardOutput.ReadToEndAsync();
            var stderr = await process.StandardError.ReadToEndAsync();
            await process.WaitForExitAsync();

            Assert.True(process.ExitCode == 0, $"extractor exited {process.ExitCode}: {stderr}");
            return JsonSerializer.Deserialize<List<ExtractorResult>>(stdout) ?? [];
        }
        finally
        {
            tempDir.Delete(recursive: true);
        }
    }

    private static string FindExtractorProject()
    {
        var dir = new DirectoryInfo(Directory.GetCurrentDirectory());
        while (dir is not null)
        {
            var direct = Path.Combine(dir.FullName, "RoslynExtractor.csproj");
            if (File.Exists(direct))
            {
                return direct;
            }

            var nested = Path.Combine(dir.FullName, "internal", "scanner", "csharp", "roslynextractor", "RoslynExtractor.csproj");
            if (File.Exists(nested))
            {
                return nested;
            }

            dir = dir.Parent;
        }

        throw new FileNotFoundException("RoslynExtractor.csproj was not found from the test working directory.");
    }

    private sealed class ExtractorResult
    {
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
}
