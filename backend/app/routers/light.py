"""Свет: поиск города, световой день сегодня, расписания ламп."""

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import LampSchedule
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


@router.get("/lamp-schedules", response_model=list[schemas.LampScheduleOut])
def list_schedules(user: CurrentUser, db: DB):
    return db.scalars(
        select(LampSchedule).where(LampSchedule.user_id == user.id).order_by(LampSchedule.plant_id, LampSchedule.start_time)
    ).all()


@router.put("/lamp-schedules", response_model=list[schemas.LampScheduleOut])
def set_schedule(body: schemas.LampScheduleSet, user: CurrentUser, db: DB):
    """Заменить расписание одной лампы (своей лампы растения или общей при plant_id=null).
    Пустой список — расписания нет, лампа только вручную."""
    if body.plant_id is not None:
        crud.owned_plant(db, user, body.plant_id)
    intervals = sorted((i.start_time, i.end_time) for i in body.intervals)
    for s, e in intervals:
        if e <= s:
            raise HTTPException(status.HTTP_400_BAD_REQUEST, "Конец интервала должен быть позже начала (через полночь — двумя интервалами)")
    for (_, e1), (s2, _) in zip(intervals, intervals[1:]):
        if s2 < e1:
            raise HTTPException(status.HTTP_400_BAD_REQUEST, "Интервалы пересекаются")
    return svc.replace_schedule(db, user.id, body.plant_id, intervals)
