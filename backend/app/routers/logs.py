"""CRUD журналов: полив, подкормка, пересадка. Принадлежат учётке через растение."""

from typing import Annotated

from fastapi import APIRouter, Depends, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import FeedingLog, Plant, RepottingLog, User, WateringLog
from app.services.plants import now_utc

router = APIRouter()
DB = Annotated[Session, Depends(get_db)]


def _list(db: Session, user: User, model, time_col, plant_id: int | None, limit: int):
    q = (
        select(model)
        .join(Plant, Plant.id == model.plant_id)
        .where(Plant.user_id == user.id)
        .order_by(time_col.desc())
        .limit(limit)
    )
    if plant_id is not None:
        q = q.where(model.plant_id == plant_id)
    return db.scalars(q).all()


# ---------- Полив ----------
@router.get("/waterings", response_model=list[schemas.WateringOut], tags=["waterings"])
def list_waterings(user: CurrentUser, db: DB, plant_id: int | None = None, limit: int = 100):
    return _list(db, user, WateringLog, WateringLog.watered_at, plant_id, limit)


@router.post("/waterings", response_model=schemas.WateringOut, status_code=status.HTTP_201_CREATED, tags=["waterings"])
def create_watering(body: schemas.WateringCreate, user: CurrentUser, db: DB):
    crud.owned_plant(db, user, body.plant_id)
    return crud.save(
        db, WateringLog(plant_id=body.plant_id, watered_at=body.watered_at or now_utc(), note=body.note)
    )


@router.get("/waterings/{log_id}", response_model=schemas.WateringOut, tags=["waterings"])
def get_watering(log_id: int, user: CurrentUser, db: DB):
    return crud.owned_log(db, user, WateringLog, log_id)


@router.patch("/waterings/{log_id}", response_model=schemas.WateringOut, tags=["waterings"])
def update_watering(log_id: int, body: schemas.WateringUpdate, user: CurrentUser, db: DB):
    obj = crud.owned_log(db, user, WateringLog, log_id)
    crud.apply_update(obj, body)
    return crud.save(db, obj)


@router.delete("/waterings/{log_id}", status_code=status.HTTP_204_NO_CONTENT, tags=["waterings"])
def delete_watering(log_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_log(db, user, WateringLog, log_id))


# ---------- Подкормка ----------
@router.get("/feedings", response_model=list[schemas.FeedingOut], tags=["feedings"])
def list_feedings(user: CurrentUser, db: DB, plant_id: int | None = None, limit: int = 100):
    return _list(db, user, FeedingLog, FeedingLog.fed_at, plant_id, limit)


@router.post("/feedings", response_model=schemas.FeedingOut, status_code=status.HTTP_201_CREATED, tags=["feedings"])
def create_feeding(body: schemas.FeedingCreate, user: CurrentUser, db: DB):
    crud.owned_plant(db, user, body.plant_id)
    crud.owned_fertilizer(db, user, body.fertilizer_type_id)
    data = body.model_dump()
    data["fed_at"] = body.fed_at or now_utc()
    return crud.save(db, FeedingLog(**data))


@router.get("/feedings/{log_id}", response_model=schemas.FeedingOut, tags=["feedings"])
def get_feeding(log_id: int, user: CurrentUser, db: DB):
    return crud.owned_log(db, user, FeedingLog, log_id)


@router.patch("/feedings/{log_id}", response_model=schemas.FeedingOut, tags=["feedings"])
def update_feeding(log_id: int, body: schemas.FeedingUpdate, user: CurrentUser, db: DB):
    obj = crud.owned_log(db, user, FeedingLog, log_id)
    if body.fertilizer_type_id is not None:
        crud.owned_fertilizer(db, user, body.fertilizer_type_id)
    crud.apply_update(obj, body)
    return crud.save(db, obj)


@router.delete("/feedings/{log_id}", status_code=status.HTTP_204_NO_CONTENT, tags=["feedings"])
def delete_feeding(log_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_log(db, user, FeedingLog, log_id))


# ---------- Пересадка ----------
@router.get("/repottings", response_model=list[schemas.RepottingOut], tags=["repottings"])
def list_repottings(user: CurrentUser, db: DB, plant_id: int | None = None, limit: int = 100):
    return _list(db, user, RepottingLog, RepottingLog.repotted_at, plant_id, limit)


@router.post("/repottings", response_model=schemas.RepottingOut, status_code=status.HTTP_201_CREATED, tags=["repottings"])
def create_repotting(body: schemas.RepottingCreate, user: CurrentUser, db: DB):
    plant = crud.owned_plant(db, user, body.plant_id)
    data = body.model_dump()
    data["repotted_at"] = body.repotted_at or now_utc()
    if data["pot_size_before"] is None:
        data["pot_size_before"] = plant.pot_size_l
    if body.pot_size_after is not None:
        plant.pot_size_l = body.pot_size_after
    return crud.save(db, RepottingLog(**data))


@router.get("/repottings/{log_id}", response_model=schemas.RepottingOut, tags=["repottings"])
def get_repotting(log_id: int, user: CurrentUser, db: DB):
    return crud.owned_log(db, user, RepottingLog, log_id)


@router.patch("/repottings/{log_id}", response_model=schemas.RepottingOut, tags=["repottings"])
def update_repotting(log_id: int, body: schemas.RepottingUpdate, user: CurrentUser, db: DB):
    obj = crud.owned_log(db, user, RepottingLog, log_id)
    crud.apply_update(obj, body)
    return crud.save(db, obj)


@router.delete("/repottings/{log_id}", status_code=status.HTTP_204_NO_CONTENT, tags=["repottings"])
def delete_repotting(log_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_log(db, user, RepottingLog, log_id))
