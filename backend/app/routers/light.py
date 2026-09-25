"""Свет: поиск города и световой день сегодня."""

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy.orm import Session

from app import schemas
from app.auth import CurrentUser
from app.db import get_db
from app.services import light as svc

router = APIRouter(tags=["light"])
DB = Annotated[Session, Depends(get_db)]


@router.get("/light/geocode", response_model=list[schemas.Place])
def geocode(_: CurrentUser, q: Annotated[str, Query(min_length=2, max_length=100)]):
    """Поиск города по названию (Open-Meteo), для выбора в настройках."""
    try:
        return svc.geocode(q)
    except OSError:
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, "Сервис поиска городов недоступен, попробуйте позже")


@router.get("/light/today", response_model=schemas.DaylightOut | None)
def today(user: CurrentUser, db: DB):
    """Свет сегодня в городе учётки; null — город не задан или данные ещё не получены."""
    return svc.today_daylight(db, user.id)
