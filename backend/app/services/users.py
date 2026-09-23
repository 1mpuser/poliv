"""Учётки: создание, пароли, блокировка. Общий код для админки и CLI."""

from datetime import datetime, timezone

from fastapi import HTTPException, status
from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.models import FertilizerType, User, UserSettings
from app.passwords import MAX_LENGTH, MIN_LENGTH, hash_password

# Заглушка владельца из миграции 0002 — ей set-owner даёт настоящую почту
PLACEHOLDER_EMAIL = "owner@localhost.invalid"

# Удобрения, с которыми начинается новая учётка (NPK и дозы — с этикетки, в настройках)
DEFAULT_FERTILIZERS = (
    {"name": "Lomonosoff", "interval_days_active_season": 14, "interval_days_dormant_season": 30},
    {"name": "Bona Forte", "interval_days_active_season": 14, "interval_days_dormant_season": 30},
)


def normalize_email(email: str) -> str:
    return email.strip().lower()


def check_password(password: str) -> None:
    if not MIN_LENGTH <= len(password) <= MAX_LENGTH:
        raise HTTPException(
            status.HTTP_400_BAD_REQUEST, f"Пароль — от {MIN_LENGTH} до {MAX_LENGTH} символов"
        )


def find_by_email(db: Session, email: str) -> User | None:
    return db.scalars(select(User).where(func.lower(User.email) == normalize_email(email))).first()


def create_user(db: Session, email: str, password: str, is_admin: bool = False) -> User:
    """Учётка + её настройки + стартовые удобрения — одной транзакцией."""
    email = normalize_email(email)
    check_password(password)
    if find_by_email(db, email):
        raise HTTPException(status.HTTP_409_CONFLICT, "Учётка с такой почтой уже есть")
    user = User(email=email, password_hash=hash_password(password), is_admin=is_admin)
    db.add(user)
    db.flush()
    db.add(UserSettings(user_id=user.id))
    db.add_all(FertilizerType(user_id=user.id, **f) for f in DEFAULT_FERTILIZERS)
    db.commit()
    db.refresh(user)
    return user


def set_password(db: Session, user: User, password: str) -> None:
    check_password(password)
    user.password_hash = hash_password(password)
    user.token_version += 1  # разлогинить все устройства
    db.commit()


def set_blocked(db: Session, user: User, blocked: bool) -> None:
    if blocked and user.blocked_at is None:
        user.blocked_at = datetime.now(timezone.utc)
        user.token_version += 1
    elif not blocked:
        user.blocked_at = None
    db.commit()
