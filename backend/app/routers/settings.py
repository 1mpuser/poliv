from typing import Annotated

from fastapi import APIRouter, Depends
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.services.plants import get_user_settings

router = APIRouter(prefix="/settings", tags=["settings"])
DB = Annotated[Session, Depends(get_db)]


@router.get("", response_model=schemas.SettingsOut)
def read_settings(user: CurrentUser, db: DB):
    return get_user_settings(db, user.id)


@router.patch("", response_model=schemas.SettingsOut)
def update_settings(body: schemas.SettingsUpdate, user: CurrentUser, db: DB):
    row = get_user_settings(db, user.id)
    crud.apply_update(row, body)
    return crud.save(db, row)
