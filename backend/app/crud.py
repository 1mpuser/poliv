from typing import TypeVar

from fastapi import HTTPException, status
from pydantic import BaseModel
from sqlalchemy.orm import Session

from app.db import Base
from app.models import FertilizerType, Lamp, LampSession, Plant, User

M = TypeVar("M", bound=Base)


def get_or_404(db: Session, model: type[M], obj_id: int, message: str = "Запись не найдена") -> M:
    obj = db.get(model, obj_id)
    if obj is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, message)
    return obj


def apply_update(obj: Base, data: BaseModel) -> None:
    """PATCH: меняем только переданные поля (явный null тоже считается)."""
    for key, value in data.model_dump(exclude_unset=True).items():
        setattr(obj, key, value)


def save(db: Session, obj: M) -> M:
    db.add(obj)
    db.commit()
    db.refresh(obj)
    return obj


def delete(db: Session, obj: Base) -> None:
    db.delete(obj)
    db.commit()


# ---------- Владение: чужая запись неотличима от несуществующей (404) ----------
def owned_plant(db: Session, user: User, plant_id: int) -> Plant:
    plant = db.get(Plant, plant_id)
    if plant is None or plant.user_id != user.id:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Растение не найдено")
    return plant


def owned_fertilizer(db: Session, user: User, fertilizer_id: int) -> FertilizerType:
    obj = db.get(FertilizerType, fertilizer_id)
    if obj is None or obj.user_id != user.id:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Удобрение не найдено")
    return obj


def owned_log(db: Session, user: User, model: type[M], log_id: int) -> M:
    """Журналы полива/подкормки/пересадки принадлежат учётке через растение."""
    obj = db.get(model, log_id)
    if obj is None or db.get(Plant, obj.plant_id).user_id != user.id:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Запись не найдена")
    return obj


def owned_lamp_session(db: Session, user: User, session_id: int) -> LampSession:
    obj = db.get(LampSession, session_id)
    if obj is None or obj.user_id != user.id:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Сессия лампы не найдена")
    return obj


def owned_lamp(db: Session, user: User, lamp_id: int) -> Lamp:
    """Архивная («удалённая») лампа тоже 404."""
    obj = db.get(Lamp, lamp_id)
    if obj is None or obj.user_id != user.id or obj.archived_at is not None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Лампа не найдена")
    return obj
