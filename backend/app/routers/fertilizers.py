from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.db import get_db
from app.models import FertilizerType

router = APIRouter(prefix="/fertilizers", tags=["fertilizers"])
DB = Annotated[Session, Depends(get_db)]


def _save(db: Session, obj: FertilizerType) -> FertilizerType:
    try:
        return crud.save(db, obj)
    except IntegrityError:
        db.rollback()
        raise HTTPException(status.HTTP_409_CONFLICT, "Удобрение с таким названием уже есть")


@router.get("", response_model=list[schemas.FertilizerOut])
def list_fertilizers(db: DB):
    return db.scalars(select(FertilizerType).order_by(FertilizerType.id)).all()


@router.post("", response_model=schemas.FertilizerOut, status_code=status.HTTP_201_CREATED)
def create_fertilizer(body: schemas.FertilizerCreate, db: DB):
    return _save(db, FertilizerType(**body.model_dump()))


@router.get("/{fertilizer_id}", response_model=schemas.FertilizerOut)
def get_fertilizer(fertilizer_id: int, db: DB):
    return crud.get_or_404(db, FertilizerType, fertilizer_id, "Удобрение не найдено")


@router.patch("/{fertilizer_id}", response_model=schemas.FertilizerOut)
def update_fertilizer(fertilizer_id: int, body: schemas.FertilizerUpdate, db: DB):
    obj = crud.get_or_404(db, FertilizerType, fertilizer_id, "Удобрение не найдено")
    crud.apply_update(obj, body)
    return _save(db, obj)


@router.delete("/{fertilizer_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_fertilizer(fertilizer_id: int, db: DB):
    # История подкормок сохраняется: fertilizer_type_id станет NULL
    crud.delete(db, crud.get_or_404(db, FertilizerType, fertilizer_id, "Удобрение не найдено"))
