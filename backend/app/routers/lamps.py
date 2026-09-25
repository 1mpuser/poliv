"""Лампы учётки: растения под лампой, режим, розетка, расписание, кнопка; устройства Яндекса."""

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import Lamp, LampMode, User
from app.services import lamps as svc
from app.services import light, yandex
from app.services.plants import get_user_settings, now_utc

router = APIRouter(prefix="/lamps", tags=["lamps"])
yandex_router = APIRouter(prefix="/yandex", tags=["lamps"])
DB = Annotated[Session, Depends(get_db)]


def _own_plant_ids(db: Session, user: User, ids: list[int]) -> list[int]:
    for pid in ids:
        crud.owned_plant(db, user, pid)
    return list(dict.fromkeys(ids))


def _check(db: Session, user: User, mode: LampMode, morning, evening) -> None:
    if evening <= morning:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "«Вечером не позже» должно быть позже «утром не раньше»")
    if mode == LampMode.auto and get_user_settings(db, user.id).latitude is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Для режима «Авто» задайте город в настройках (раздел «Свет»)")


def toggle_out(db: Session, lamp: Lamp) -> schemas.LampToggleOut:
    try:
        res = svc.toggle(db, lamp, now_utc())
    except IntegrityError:
        db.rollback()
        raise HTTPException(status.HTTP_409_CONFLICT, "Лампа уже горит, или время выключения раньше включения")
    return schemas.LampToggleOut(
        is_on=res.is_on,
        session=schemas.LampSessionOut.model_validate(res.session),
        previous_ended_at=res.previous_ended_at,
        plug_error=lamp.last_error,
    )


@router.get("", response_model=list[schemas.LampOut])
def list_lamps(user: CurrentUser, db: DB):
    now = now_utc()
    q = select(Lamp).where(Lamp.user_id == user.id, Lamp.archived_at.is_(None)).order_by(Lamp.id)
    return [svc.lamp_out(db, lamp, now) for lamp in db.scalars(q)]


@router.post("", response_model=schemas.LampOut, status_code=status.HTTP_201_CREATED)
def create_lamp(body: schemas.LampCreate, user: CurrentUser, db: DB):
    _check(db, user, body.mode, body.morning_not_before, body.evening_not_after)
    ids = _own_plant_ids(db, user, body.plant_ids)
    lamp = Lamp(user_id=user.id, **body.model_dump(exclude={"plant_ids"}))
    db.add(lamp)
    db.flush()
    now = now_utc()
    svc.set_plants(db, lamp, ids, now)
    db.commit()
    svc.after_change(db, lamp, now)
    return svc.lamp_out(db, lamp, now)


@router.get("/{lamp_id}", response_model=schemas.LampOut)
def get_lamp(lamp_id: int, user: CurrentUser, db: DB):
    return svc.lamp_out(db, crud.owned_lamp(db, user, lamp_id), now_utc())


@router.patch("/{lamp_id}", response_model=schemas.LampOut)
def update_lamp(lamp_id: int, body: schemas.LampUpdate, user: CurrentUser, db: DB):
    lamp = crud.owned_lamp(db, user, lamp_id)
    data = body.model_dump(exclude_unset=True)
    ids = data.pop("plant_ids", None)
    _check(
        db, user,
        data.get("mode", lamp.mode),
        data.get("morning_not_before", lamp.morning_not_before),
        data.get("evening_not_after", lamp.evening_not_after),
    )
    if ids is not None:
        ids = _own_plant_ids(db, user, ids)
    now = now_utc()
    svc.update(db, lamp, data, ids, now)
    return svc.lamp_out(db, lamp, now)


@router.delete("/{lamp_id}", status_code=status.HTTP_204_NO_CONTENT)
def archive_lamp(lamp_id: int, user: CurrentUser, db: DB):
    """Лампа уходит в архив: растения остаются без лампы, часы в их истории сохраняются."""
    svc.archive(db, crud.owned_lamp(db, user, lamp_id), now_utc())


@router.put("/{lamp_id}/schedule", response_model=list[schemas.ScheduleInterval])
def set_schedule(lamp_id: int, body: schemas.LampScheduleSet, user: CurrentUser, db: DB):
    """Полная замена расписания; пустой список — расписания нет."""
    lamp = crud.owned_lamp(db, user, lamp_id)
    intervals = light.validate_intervals(body.intervals)
    if lamp.mode != LampMode.schedule and intervals:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Расписание задаётся в режиме «По расписанию»")
    rows = light.replace_schedule(db, user.id, lamp.id, intervals)
    svc.sync_plug(db, lamp, now_utc())
    return [schemas.ScheduleInterval(start_time=r.start_time, end_time=r.end_time) for r in rows]


@router.post("/{lamp_id}/toggle", response_model=schemas.LampToggleOut)
def toggle(lamp_id: int, user: CurrentUser, db: DB):
    return toggle_out(db, crud.owned_lamp(db, user, lamp_id))


@yandex_router.get("/devices", response_model=list[schemas.YandexDevice])
def devices(user: CurrentUser, db: DB):
    """Устройства Умного дома, которые умеют вкл/выкл, — для выбора розетки лампы."""
    token = svc.yandex_token(db, user.id)
    if token is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Сначала вставьте токен Яндекса в разделе «Свет»")
    try:
        return [schemas.YandexDevice(id=d.id, name=d.name, room=d.room, type=d.type) for d in yandex.list_devices(token)]
    except yandex.YandexAuthError:
        get_user_settings(db, user.id).yandex_token_invalid = True
        db.commit()
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Токен Яндекса недействителен — вставьте новый в разделе «Свет»")
    except yandex.YandexError as e:
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, str(e))
