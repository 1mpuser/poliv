import hmac
import time
from datetime import datetime, timedelta, timezone
from typing import Annotated

import jwt
from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm

from app.config import settings
from app.schemas import Token

ALGORITHM = "HS256"
oauth2 = OAuth2PasswordBearer(tokenUrl="/api/auth/token")
router = APIRouter(prefix="/auth", tags=["auth"])


def _eq(a: str, b: str) -> bool:
    return hmac.compare_digest(a.encode(), b.encode())


@router.post("/token", response_model=Token)
def login(form: Annotated[OAuth2PasswordRequestForm, Depends()]) -> Token:
    ok_user = _eq(form.username, settings.app_username)
    ok_pass = _eq(form.password, settings.app_password)
    if not (ok_user and ok_pass):
        time.sleep(1)  # притормаживаем перебор
        raise HTTPException(status.HTTP_401_UNAUTHORIZED, "Неверный логин или пароль")
    exp = datetime.now(timezone.utc) + timedelta(days=settings.jwt_expire_days)
    token = jwt.encode({"sub": form.username, "exp": exp}, settings.jwt_secret, ALGORITHM)
    return Token(access_token=token)


def require_user(token: Annotated[str, Depends(oauth2)]) -> str:
    try:
        payload = jwt.decode(token, settings.jwt_secret, algorithms=[ALGORITHM])
    except jwt.PyJWTError:
        raise HTTPException(
            status.HTTP_401_UNAUTHORIZED,
            "Сессия истекла, войдите снова",
            headers={"WWW-Authenticate": "Bearer"},
        )
    return payload["sub"]


@router.get("/me")
def me(user: Annotated[str, Depends(require_user)]) -> dict[str, str]:
    return {"username": user}
