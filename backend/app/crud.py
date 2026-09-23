from typing import TypeVar

from fastapi import HTTPException, status
from pydantic import BaseModel
from sqlalchemy.orm import Session

from app.db import Base

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
