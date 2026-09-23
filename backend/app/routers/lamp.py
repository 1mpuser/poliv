from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.db import get_db
from app.models import LampSession, Plant
from app.services.plants import now_utc

router = APIRouter(prefix="/lamp-sessions", tags=["lamp"])
DB = Annotated[Session, Depends(get_db)]

SHARED_LAMP_DEFAULT_HOURS = 12.0


def _planned_hours(db: Session, plant_id: int | None) -> float:
    if plant_id is None:
        return SHARED_LAMP_DEFAULT_HOURS
    return crud.get_or_404(db, Plant, plant_id, "Растение не найдено").lamp_hours_per_day


def _open_session(db: Session, plant_id: int | None) -> LampSession | None:
    cond = LampSession.plant_id.is_(None) if plant_id is None else LampSession.plant_id == plant_id
    return db.scalars(select(LampSession).where(cond, LampSession.ended_at.is_(None))).first()


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
    db: DB,
    plant_id: int | None = None,
    shared: bool = False,
    open: bool | None = None,
    limit: int = 100,
):
    """plant_id — сессии растения; shared=true — только общая лампа; open=true — горящие сейчас."""
    q = select(LampSession).order_by(LampSession.started_at.desc()).limit(limit)
    if shared:
        q = q.where(LampSession.plant_id.is_(None))
    elif plant_id is not None:
        q = q.where(LampSession.plant_id == plant_id)
    if open is not None:
        q = q.where(LampSession.ended_at.is_(None) if open else LampSession.ended_at.is_not(None))
    return db.scalars(q).all()


@router.post("", response_model=schemas.LampSessionOut, status_code=status.HTTP_201_CREATED)
def create_session(body: schemas.LampSessionCreate, db: DB):
    planned = body.planned_hours_per_day
    if planned is None:
        planned = _planned_hours(db, body.plant_id)
    elif body.plant_id is not None:
        crud.get_or_404(db, Plant, body.plant_id, "Растение не найдено")
    return _save(
        db,
        LampSession(
            plant_id=body.plant_id,
            started_at=body.started_at or now_utc(),
            ended_at=body.ended_at,
            planned_hours_per_day=planned,
        ),
    )


@router.post("/toggle", response_model=schemas.LampToggleOut)
def toggle(body: schemas.LampToggle, db: DB):
    """Выключает горящую лампу растения (или общую при plant_id=null), иначе включает."""
    current = _open_session(db, body.plant_id)
    if current is not None:
        current.ended_at = now_utc()
        return schemas.LampToggleOut(is_on=False, session=_save(db, current))
    session = LampSession(
        plant_id=body.plant_id,
        started_at=now_utc(),
        planned_hours_per_day=_planned_hours(db, body.plant_id),
    )
    return schemas.LampToggleOut(is_on=True, session=_save(db, session))


@router.get("/{session_id}", response_model=schemas.LampSessionOut)
def get_session(session_id: int, db: DB):
    return crud.get_or_404(db, LampSession, session_id, "Сессия лампы не найдена")


@router.patch("/{session_id}", response_model=schemas.LampSessionOut)
def update_session(session_id: int, body: schemas.LampSessionUpdate, db: DB):
    """ended_at=null снова «зажигает» сессию — так работает отмена выключения."""
    obj = crud.get_or_404(db, LampSession, session_id, "Сессия лампы не найдена")
    crud.apply_update(obj, body)
    return _save(db, obj)


@router.delete("/{session_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_session(session_id: int, db: DB):
    crud.delete(db, crud.get_or_404(db, LampSession, session_id, "Сессия лампы не найдена"))
