import time
from datetime import datetime, timedelta, timezone
from typing import Annotated

import jwt
from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm
from pydantic import BaseModel
from sqlalchemy.orm import Session

from app.config import settings
from app.db import get_db
from app.models import User
from app.passwords import verify_password
from app.schemas import Token
from app.services import users as users_svc

ALGORITHM = "HS256"
oauth2 = OAuth2PasswordBearer(tokenUrl="/api/auth/token")
router = APIRouter(prefix="/auth", tags=["auth"])
DB = Annotated[Session, Depends(get_db)]

UNAUTHORIZED = HTTPException(
    status.HTTP_401_UNAUTHORIZED,
    "Сессия истекла, войдите снова",
    headers={"WWW-Authenticate": "Bearer"},
)


def issue_token(user: User) -> str:
    exp = datetime.now(timezone.utc) + timedelta(days=settings.jwt_expire_days)
    payload = {"sub": str(user.id), "ver": user.token_version, "exp": exp}
    return jwt.encode(payload, settings.jwt_secret, ALGORITHM)


@router.post("/token", response_model=Token)
def login(form: Annotated[OAuth2PasswordRequestForm, Depends()], db: DB) -> Token:
    """username — почта. Заблокированная учётка получает тот же ответ, что и неверный пароль."""
    user = users_svc.find_by_email(db, form.username)
    ok = user is not None and verify_password(form.password, user.password_hash) and user.blocked_at is None
    if not ok:
        time.sleep(1)  # притормаживаем перебор
        raise HTTPException(status.HTTP_401_UNAUTHORIZED, "Неверная почта или пароль")
    return Token(access_token=issue_token(user))


def current_user(token: Annotated[str, Depends(oauth2)], db: DB) -> User:
    """Токен действителен, пока учётка не заблокирована и не менялся пароль (token_version)."""
    try:
        payload = jwt.decode(token, settings.jwt_secret, algorithms=[ALGORITHM])
        user = db.get(User, int(payload["sub"]))
    except (jwt.PyJWTError, KeyError, ValueError):
        raise UNAUTHORIZED
    if user is None or user.blocked_at is not None or payload.get("ver") != user.token_version:
        raise UNAUTHORIZED
    return user


CurrentUser = Annotated[User, Depends(current_user)]


def require_admin(user: CurrentUser) -> User:
    # 404, а не 403: наличие админки не раскрывается
    if not user.is_admin:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Not Found")
    return user


class Me(BaseModel):
    id: int
    email: str
    is_admin: bool


class PasswordChange(BaseModel):
    current_password: str
    new_password: str


@router.get("/me", response_model=Me)
def me(user: CurrentUser):
    return user


@router.post("/password", response_model=Token)
def change_password(body: PasswordChange, user: CurrentUser, db: DB) -> Token:
    """Смена своего пароля. Остальные устройства разлогиниваются, это получает новый токен."""
    if not verify_password(body.current_password, user.password_hash):
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Текущий пароль неверен")
    users_svc.set_password(db, user, body.new_password)
    return Token(access_token=issue_token(user))
