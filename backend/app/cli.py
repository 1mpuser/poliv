"""Управление учётками с сервера.

    docker compose exec backend python -m app.cli set-owner --email you@example.com
    docker compose exec backend python -m app.cli create-user --email friend@example.com
    docker compose exec backend python -m app.cli reset-password --email you@example.com
    docker compose exec backend python -m app.cli make-admin --email friend@example.com

Пароль генерируется и печатается один раз (или задаётся --password).
"""

import argparse
import sys

from sqlalchemy import select

from app.db import SessionLocal
from app.models import User
from app.passwords import generate_password
from app.services import users as svc


def _print_credentials(email: str, password: str) -> None:
    print(f"Почта:  {email}\nПароль: {password}\nСохраните пароль — повторно он не показывается.")


def set_owner(email: str, password: str) -> None:
    """Заглушке владельца из миграции (или существующей учётке) — почту, пароль и права админа."""
    email = svc.normalize_email(email)
    with SessionLocal() as db:
        user = svc.find_by_email(db, email) or db.scalars(
            select(User).where(User.email == svc.PLACEHOLDER_EMAIL)
        ).first()
        if user is None:
            svc.create_user(db, email, password, is_admin=True)
        else:
            user.email = email
            user.is_admin = True
            user.blocked_at = None
            svc.set_password(db, user, password)
    _print_credentials(email, password)


def create_user(email: str, password: str, admin: bool) -> None:
    with SessionLocal() as db:
        user = svc.create_user(db, email, password, is_admin=admin)
    _print_credentials(user.email, password)


def reset_password(email: str, password: str) -> None:
    with SessionLocal() as db:
        user = svc.find_by_email(db, email)
        if user is None:
            sys.exit(f"Учётка {email} не найдена")
        svc.set_password(db, user, password)
    _print_credentials(svc.normalize_email(email), password)


def make_admin(email: str) -> None:
    with SessionLocal() as db:
        user = svc.find_by_email(db, email)
        if user is None:
            sys.exit(f"Учётка {email} не найдена")
        user.is_admin = True
        db.commit()
    print(f"{user.email} теперь админ")


def main() -> None:
    parser = argparse.ArgumentParser(prog="python -m app.cli", description="Учётки Поливалки")
    sub = parser.add_subparsers(dest="cmd", required=True)
    for name in ("set-owner", "create-user", "reset-password", "make-admin"):
        p = sub.add_parser(name)
        p.add_argument("--email", required=True)
        if name != "make-admin":
            p.add_argument("--password", help="по умолчанию генерируется")
        if name == "create-user":
            p.add_argument("--admin", action="store_true")
    args = parser.parse_args()
    password = getattr(args, "password", None) or generate_password()

    try:
        if args.cmd == "set-owner":
            set_owner(args.email, password)
        elif args.cmd == "create-user":
            create_user(args.email, password, args.admin)
        elif args.cmd == "reset-password":
            reset_password(args.email, password)
        else:
            make_admin(args.email)
    except Exception as e:  # HTTPException из сервиса — показать текст, а не трейс
        sys.exit(getattr(e, "detail", None) or str(e))


if __name__ == "__main__":
    main()
