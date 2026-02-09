from sqlalchemy import text, create_engine
from sqlalchemy.orm import Session

engine = create_engine("postgresql://localhost/mydb")

with Session(engine) as session:
    # Should be extracted: text() with SQL.
    result = session.execute(text("SELECT * FROM users"))

    # Should be extracted: direct .execute() with SQL string.
    session.execute("DELETE FROM sessions WHERE expired = true")

    # Should be extracted: triple-quoted text().
    result = session.execute(text("""
        SELECT id, name
        FROM users
        WHERE active = true
    """))

    # valk-guard:disable VG001
    result = session.execute(text("SELECT * FROM orders"))

    # Should NOT be extracted: text() inside filter (fragment — no match).
    query = session.query(User).filter(text("age > 18"))
