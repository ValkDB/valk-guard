using Microsoft.EntityFrameworkCore;

namespace Demo;

public sealed class DangerousQueries
{
    public void Run(DbContext db)
    {
        // No raw .sql file here - just EF Core calls.
        // Valk Guard runs a Roslyn AST extractor and catches the problems.

        db.Database.ExecuteSqlRaw("SELECT * FROM users");

        db.Database.ExecuteSqlRaw("UPDATE orders SET status = 'cancelled'");

        db.Database.ExecuteSqlRaw("DELETE FROM sessions");

        db.Users
            .Where(user => user.Email.Contains("@gmail.com"))
            .ToList();
    }
}
