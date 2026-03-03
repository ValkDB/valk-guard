from sqlalchemy import create_engine, select, text
from sqlalchemy.orm import sessionmaker
from models import User

engine = create_engine("postgresql://localhost/example")
Session = sessionmaker(bind=engine)
session = Session()

# VG001: select-star (ORM chain)
users = session.query(User).all()

# VG002: missing-where-update (ORM chain)
session.query(User).update({"active": False})

# VG003: missing-where-delete (ORM chain)
session.query(User).delete()

# VG004: unbounded-select (ORM chain)
session.query(User).filter(User.active == True).all()

# VG005: like-leading-wildcard (ORM chain)
session.query(User).filter(User.email.like("%@example.com")).all()

# VG006: select-for-update-no-where (ORM chain; requires FOR UPDATE synthesis)
session.query(User).with_for_update().all()

# VG007: destructive-ddl (raw SQL in SQLAlchemy)
session.execute(text("DROP TABLE archived_users"))

# VG008: non-concurrent-index (raw SQL in SQLAlchemy)
session.execute(text("CREATE INDEX idx_users_email ON users(email)"))

# Valid query (Core style with explicit columns and limit)
stmt = select(User.id, User.email).where(User.active == True).limit(10)
session.execute(stmt)

# Inline Disable
# valk-guard:disable VG001
session.execute(text("SELECT * FROM legacy_table"))
