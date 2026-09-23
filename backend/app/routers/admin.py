"""Раздел админа: выдача учёток. Для не-админа все пути отвечают 404."""

from datetime import datetime
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel, ConfigDict
from sqlalchemy import select
from sqlalchemy.orm import Session

from app import crud
from app.auth import require_admin
from app.db import get_db
from app.models import User
from app.services import users as users_svc

router = APIRouter(prefix="/admin", tags=["admin"])
DB = Annotated[Session, Depends(get_db)]
Admin = Annotated[User, Depends(require_admin)]


class UserOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    email: str
    is_admin: bool
    blocked_at: datetime | None
    created_at: datetime


class UserCreate(BaseModel):
    email: str
    password: str


class PasswordSet(BaseModel):
    password: str


def _user(db: Session, user_id: int) -> User:
    return crud.get_or_404(db, User, user_id, "Учётка не найдена")


def _not_self(admin: User, user: User) -> None:
    if admin.id == user.id:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Нельзя заблокировать или удалить свою учётку")


@router.get("/users", response_model=list[UserOut])
def list_users(_: Admin, db: DB):
    return db.scalars(select(User).order_by(User.created_at, User.id)).all()


@router.post("/users", response_model=UserOut, status_code=status.HTTP_201_CREATED)
def create_user(body: UserCreate, _: Admin, db: DB):
    if "@" not in body.email or len(body.email.strip()) > 254:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Укажите почту")
    return users_svc.create_user(db, body.email, body.password)


@router.post("/users/{user_id}/password", status_code=status.HTTP_204_NO_CONTENT)
def set_password(user_id: int, body: PasswordSet, _: Admin, db: DB):
    users_svc.set_password(db, _user(db, user_id), body.password)


@router.post("/users/{user_id}/block", status_code=status.HTTP_204_NO_CONTENT)
def block(user_id: int, admin: Admin, db: DB):
    user = _user(db, user_id)
    _not_self(admin, user)
    users_svc.set_blocked(db, user, True)


@router.post("/users/{user_id}/unblock", status_code=status.HTTP_204_NO_CONTENT)
def unblock(user_id: int, _: Admin, db: DB):
    users_svc.set_blocked(db, _user(db, user_id), False)


@router.delete("/users/{user_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_user(user_id: int, admin: Admin, db: DB):
    """Удаляет учётку со всеми растениями, журналами и настройками (ON DELETE CASCADE)."""
    user = _user(db, user_id)
    _not_self(admin, user)
    crud.delete(db, user)
