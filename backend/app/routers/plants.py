from datetime import date
from typing import Annotated

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import Plant
from app.services import lamps as lamps_svc
from app.services import plants as svc

router = APIRouter(prefix="/plants", tags=["plants"])
DB = Annotated[Session, Depends(get_db)]


def _my_plants(db: Session, user_id: int):
    return db.scalars(select(Plant).where(Plant.user_id == user_id).order_by(Plant.id)).all()


@router.get("", response_model=list[schemas.PlantOut])
def list_plants(user: CurrentUser, db: DB):
    return _my_plants(db, user.id)


@router.post("", response_model=schemas.PlantOut, status_code=status.HTTP_201_CREATED)
def create_plant(body: schemas.PlantCreate, user: CurrentUser, db: DB):
    return crud.save(db, Plant(user_id=user.id, **body.model_dump()))


@router.get("/summary", response_model=list[schemas.PlantSummary])
def all_summaries(user: CurrentUser, db: DB):
    """Сводки всех растений учётки одним запросом — для дашборда."""
    return svc.build_summaries(db, user, _my_plants(db, user.id))


@router.get("/{plant_id}", response_model=schemas.PlantOut)
def get_plant(plant_id: int, user: CurrentUser, db: DB):
    return crud.owned_plant(db, user, plant_id)


@router.patch("/{plant_id}", response_model=schemas.PlantOut)
def update_plant(plant_id: int, body: schemas.PlantUpdate, user: CurrentUser, db: DB):
    plant = crud.owned_plant(db, user, plant_id)
    crud.apply_update(plant, body)
    return crud.save(db, plant)


@router.delete("/{plant_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_plant(plant_id: int, user: CurrentUser, db: DB):
    crud.delete(db, crud.owned_plant(db, user, plant_id))


@router.get("/{plant_id}/summary", response_model=schemas.PlantSummary)
def plant_summary(plant_id: int, user: CurrentUser, db: DB):
    return svc.build_summaries(db, user, [crud.owned_plant(db, user, plant_id)])[0]


@router.get("/{plant_id}/history", response_model=list[schemas.HistoryEvent])
def plant_history(
    plant_id: int,
    user: CurrentUser,
    db: DB,
    types: Annotated[list[schemas.EventType] | None, Query(description="Фильтр по типу, можно несколько")] = None,
    date_from: date | None = None,
    date_to: date | None = None,
):
    plant = crud.owned_plant(db, user, plant_id)
    wanted = set(types or ["water", "feed", "lamp", "repot"])
    return svc.history(db, plant, wanted, date_from, date_to)


@router.get("/{plant_id}/stats/weekly", response_model=list[schemas.WeekStatOut])
def plant_weekly_stats(
    plant_id: int, user: CurrentUser, db: DB, weeks: Annotated[int, Query(ge=1, le=52)] = 8
):
    return svc.weekly(db, crud.owned_plant(db, user, plant_id), weeks)


@router.put("/{plant_id}/lamp", response_model=schemas.PlantLampSet)
def set_plant_lamp(plant_id: int, body: schemas.PlantLampSet, user: CurrentUser, db: DB):
    """Перенести растение под другую лампу (null — без лампы). Прошлые часы не меняются."""
    plant = crud.owned_plant(db, user, plant_id)
    if body.lamp_id is not None:
        crud.owned_lamp(db, user, body.lamp_id)
    lamps_svc.move_plant(db, plant.id, body.lamp_id, svc.now_utc())
    return body
