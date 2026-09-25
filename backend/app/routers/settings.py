from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.config import settings as config
from app.db import get_db
from app.services import light, secret_box, yandex
from app.services.plants import get_user_settings

router = APIRouter(prefix="/settings", tags=["settings"])
DB = Annotated[Session, Depends(get_db)]


@router.get("", response_model=schemas.SettingsOut)
def read_settings(user: CurrentUser, db: DB):
    return get_user_settings(db, user.id)


@router.patch("", response_model=schemas.SettingsOut)
def update_settings(body: schemas.SettingsUpdate, user: CurrentUser, db: DB):
    row = get_user_settings(db, user.id)
    place = (row.latitude, row.longitude)
    crud.apply_update(row, body)
    row = crud.save(db, row)
    if (row.latitude, row.longitude) != place:
        # новый город — свет по нему сразу; сеть подвела — догонит фоновая синхронизация
        try:
            light.sync_daylight(db, row, force=True)
        except OSError:
            db.rollback()
    return row


@router.put("/yandex-token", response_model=schemas.SettingsOut)
def set_yandex_token(body: schemas.YandexTokenSet, user: CurrentUser, db: DB):
    """Сохранить токен Умного дома (сразу проверяется у Яндекса) или удалить (null). Токен не возвращается."""
    row = get_user_settings(db, user.id)
    if body.token is None:
        row.yandex_token, row.yandex_token_invalid = None, False
        return crud.save(db, row)
    token = body.token.strip()
    try:
        yandex.list_devices(token)
    except yandex.YandexAuthError:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Яндекс не принял токен — проверьте права «Умный дом» (iot:view, iot:control)")
    except yandex.YandexError as e:
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, str(e))
    row.yandex_token = secret_box.encrypt(token, config.jwt_secret)
    row.yandex_token_invalid = False
    return crud.save(db, row)
