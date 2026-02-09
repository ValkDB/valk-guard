from sqlalchemy import create_engine, select, text, update
from sqlalchemy.orm import sessionmaker
from models import User

engine = create_engine("postgresql://user:pass@localhost/db")
Session = sessionmaker(bind=engine)
session = Session()

# VG001: ORM Select Star (session.query defaults to selecting all columns)
users = session.query(User).all()

# VG001: Raw SQL Select Star
session.execute(text("SELECT * FROM users"))

# VG005: Invalid LIKE pattern in filter
session.query(User).filter(User.email.like("%@example.com")).all()

# VG003: Delete without filter
session.query(User).delete()

# Valid Query (Core style with explicit columns and limit)
stmt = select(User.id, User.email).where(User.active == True).limit(10)
session.execute(stmt)

# Inline Disable
# valk-guard:disable VG001
session.execute(text("SELECT * FROM legacy_table"))
