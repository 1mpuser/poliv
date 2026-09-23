from typing import Annotated

from fastapi import APIRouter, Depends
from sqlalchemy.orm import Session

from app import crud, schemas
from app.db import get_db
from app.services.plants import get_app_settings

router = APIRouter(prefix="/settings", tags=["settings"])
DB = Annotated[Session, Depends(get_db)]


@router.get("", response_model=schemas.SettingsOut)
def read_settings(db: DB):
    return get_app_settings(db)


@router.patch("", response_model=schemas.SettingsOut)
def update_settings(body: schemas.SettingsUpdate, db: DB):
    row = get_app_settings(db)
    crud.apply_update(row, body)
    return crud.save(db, row)
