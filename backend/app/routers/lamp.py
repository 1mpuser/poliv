from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import LampSession, User
from app.services.plants import covering_session, now_utc

router = APIRouter(prefix="/lamp-sessions", tags=["lamp"])
DB = Annotated[Session, Depends(get_db)]

SHARED_LAMP_DEFAULT_HOURS = 12.0


def _planned_hours(db: Session, user: User, plant_id: int | None) -> float:
    if plant_id is None:
        return SHARED_LAMP_DEFAULT_HOURS
    return crud.owned_plant(db, user, plant_id).light_target_hours


def _save(db: Session, obj: LampSession) -> LampSession:
    try:
        return crud.save(db, obj)
    except IntegrityError:
        db.rollback()
        raise HTTPException(
            status.HTTP_409_CONFLICT,
            "Лампа уже горит, или время выключения раньше включения",
        )


@router.get("", response_model=list[schemas.LampSessionOut])
def list_sessions(
    user: CurrentUser,
    db: DB,
    plant_id: int | None = None,
    shared: bool = False,
    open: bool | None = None,
    limit: int = 100,
):
    """plant_id — сессии растения; shared=true — только общая лампа; open=true — горящие сейчас."""
    q = (
        select(LampSession)
        .where(LampSession.user_id == user.id)
        .order_by(LampSession.started_at.desc())
        .limit(limit)
    )
    if shared:
        q = q.where(LampSession.plant_id.is_(None))
    elif plant_id is not None:
        q = q.where(LampSession.plant_id == plant_id)
    if open is not None:
        q = q.where(LampSession.ended_at.is_(None) if open else LampSession.ended_at.is_not(None))
    return db.scalars(q).all()


@router.post("", response_model=schemas.LampSessionOut, status_code=status.HTTP_201_CREATED)
def create_session(body: schemas.LampSessionCreate, user: CurrentUser, db: DB):
    planned = _planned_hours(db, user, body.plant_id)  # заодно проверяет владельца растения
    if body.planned_hours_per_day is not None:
        planned = body.planned_hours_per_day
    return _save(
        db,
        LampSession(
            user_id=user.id,
            plant_id=body.plant_id,
            started_at=body.started_at or now_utc(),
            ended_at=body.ended_at,
            planned_hours_per_day=planned,
        ),
    )


@router.post("/toggle", response_model=schemas.LampToggleOut)
def toggle(body: schemas.LampToggle, user: CurrentUser, db: DB):
    """Гасит то, что горит сейчас (включённую вручную или идущую по расписанию сессию),
    иначе включает вручную. previous_ended_at нужен для отмены выключения."""
    planned = _planned_hours(db, user, body.plant_id)
    now = now_utc()
    current = covering_session(db, user.id, body.plant_id, now)
    if current is not None:
        previous = current.ended_at
        current.ended_at = now
        return schemas.LampToggleOut(is_on=False, session=_save(db, current), previous_ended_at=previous)
    session = LampSession(
        user_id=user.id, plant_id=body.plant_id, started_at=now, planned_hours_per_day=planned
    )
    return schemas.LampToggleOut(is_on=True, session=_save(db, session))


@router.get("/{session_id}", response_model=schemas.LampSessionOut)
def get_session(session_id: int, user: CurrentUser, db: DB):
    return crud.owned_lamp(db, user, session_id)


@router.patch("/{session_id}", response_model=schemas.LampSessionOut)
def update_session(session_id: int, body: schemas.LampSessionUpdate, user: CurrentUser, db: DB):
    """ended_at=null снова «зажигает» сессию — так работает отмена выключения."""
    obj = crud.owned_lamp(db, user, session_id)
    crud.apply_update(obj, body)
    return _save(db, obj)


@router.delete("/{session_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_session(session_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_lamp(db, user, session_id))
