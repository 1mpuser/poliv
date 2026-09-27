"""CRUD проверок грунта. Проверка — «хозяин потрогал землю», сбрасывает счётчик растения как полив."""

from typing import Annotated

from fastapi import APIRouter, Depends, status
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import SoilCheck
from app.services.plants import now_utc

router = APIRouter()
DB = Annotated[Session, Depends(get_db)]


@router.post("/plants/{plant_id}/checks", response_model=schemas.SoilCheckOut, status_code=status.HTTP_201_CREATED)
def create_check(plant_id: int, user: CurrentUser, db: DB):
    crud.owned_plant(db, user, plant_id)
    return crud.save(db, SoilCheck(plant_id=plant_id, checked_at=now_utc()))


@router.delete("/checks/{check_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_check(check_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_log(db, user, SoilCheck, check_id))
