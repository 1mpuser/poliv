from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import Lamp, LampSession
from app.routers.lamps import toggle_out
from app.services import lamps as lamps_svc
from app.services.plants import now_utc

router = APIRouter(prefix="/lamp-sessions", tags=["lamp"])
DB = Annotated[Session, Depends(get_db)]


def _save(db: Session, obj: LampSession) -> LampSession:
    try:
        obj = crud.save(db, obj)
    except IntegrityError:
        db.rollback()
        raise HTTPException(status.HTTP_409_CONFLICT, "Лампа уже горит, или время выключения раньше включения")
    lamps_svc.sync_plug(db, db.get(Lamp, obj.lamp_id), now_utc())
    return obj


@router.get("", response_model=list[schemas.LampSessionOut])
def list_sessions(user: CurrentUser, db: DB, lamp_id: int | None = None, open: bool | None = None, limit: int = 100):
    """lamp_id — сессии одной лампы; open=true — горящие сейчас."""
    q = select(LampSession).where(LampSession.user_id == user.id).order_by(LampSession.started_at.desc()).limit(limit)
    if lamp_id is not None:
        q = q.where(LampSession.lamp_id == lamp_id)
    if open is not None:
        q = q.where(LampSession.ended_at.is_(None) if open else LampSession.ended_at.is_not(None))
    return db.scalars(q).all()


@router.post("", response_model=schemas.LampSessionOut, status_code=status.HTTP_201_CREATED)
def create_session(body: schemas.LampSessionCreate, user: CurrentUser, db: DB):
    lamp = crud.owned_lamp(db, user, body.lamp_id)
    session = LampSession(
        user_id=user.id, lamp_id=lamp.id, started_at=body.started_at or now_utc(), ended_at=body.ended_at
    )
    if body.planned_hours_per_day is not None:
        session.planned_hours_per_day = body.planned_hours_per_day
    return _save(db, session)


@router.post("/toggle", response_model=schemas.LampToggleOut)
def toggle(body: schemas.LampToggle, user: CurrentUser, db: DB):
    """Кнопка «Лампа» у растения — переключает его лампу (и соседей по ней)."""
    plant = crud.owned_plant(db, user, body.plant_id)
    lamp_id = lamps_svc.current_lamp_id(db, plant.id)
    if lamp_id is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "У растения нет лампы — привяжите её в настройках")
    return toggle_out(db, db.get(Lamp, lamp_id))


@router.get("/{session_id}", response_model=schemas.LampSessionOut)
def get_session(session_id: int, user: CurrentUser, db: DB):
    return crud.owned_lamp_session(db, user, session_id)


@router.patch("/{session_id}", response_model=schemas.LampSessionOut)
def update_session(session_id: int, body: schemas.LampSessionUpdate, user: CurrentUser, db: DB):
    """ended_at=null снова «зажигает» сессию — так работает отмена выключения."""
    obj = crud.owned_lamp_session(db, user, session_id)
    crud.apply_update(obj, body)
    return _save(db, obj)


@router.delete("/{session_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_session(session_id: int, user: CurrentUser, db: DB):
    obj = crud.owned_lamp_session(db, user, session_id)
    lamp = db.get(Lamp, obj.lamp_id)
    crud.delete(db, obj)
    lamps_svc.sync_plug(db, lamp, now_utc())
