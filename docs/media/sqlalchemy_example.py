from sqlalchemy.orm import Session
from models import User

def dangerous_queries(session: Session):
    # No raw SQL here — just SQLAlchemy ORM chains.
    # Valk Guard parses the Python AST and catches the problems.

    session.query(User).all()

    session.query(User).update({"status": "cancelled"})

    session.query(User).delete()

    session.query(User).filter(User.email.like("%@gmail.com")).all()
