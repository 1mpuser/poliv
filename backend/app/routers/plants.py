from datetime import date
from typing import Annotated

from fastapi import APIRouter, Depends, Query, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from app import crud, schemas
from app.db import get_db
from app.models import Plant
from app.services import plants as svc

router = APIRouter(prefix="/plants", tags=["plants"])
DB = Annotated[Session, Depends(get_db)]


def _plant(db: Session, plant_id: int) -> Plant:
    return crud.get_or_404(db, Plant, plant_id, "Растение не найдено")


@router.get("", response_model=list[schemas.PlantOut])
def list_plants(db: DB):
    return db.scalars(select(Plant).order_by(Plant.id)).all()


@router.post("", response_model=schemas.PlantOut, status_code=status.HTTP_201_CREATED)
def create_plant(body: schemas.PlantCreate, db: DB):
    return crud.save(db, Plant(**body.model_dump()))


@router.get("/summary", response_model=list[schemas.PlantSummary])
def all_summaries(db: DB):
    """Сводки всех растений одним запросом — для дашборда."""
    return svc.build_summaries(db, db.scalars(select(Plant).order_by(Plant.id)).all())


@router.get("/{plant_id}", response_model=schemas.PlantOut)
def get_plant(plant_id: int, db: DB):
    return _plant(db, plant_id)


@router.patch("/{plant_id}", response_model=schemas.PlantOut)
def update_plant(plant_id: int, body: schemas.PlantUpdate, db: DB):
    plant = _plant(db, plant_id)
    crud.apply_update(plant, body)
    return crud.save(db, plant)


@router.delete("/{plant_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_plant(plant_id: int, db: DB):
    crud.delete(db, _plant(db, plant_id))


@router.get("/{plant_id}/summary", response_model=schemas.PlantSummary)
def plant_summary(plant_id: int, db: DB):
    return svc.build_summaries(db, [_plant(db, plant_id)])[0]


@router.get("/{plant_id}/history", response_model=list[schemas.HistoryEvent])
def plant_history(
    plant_id: int,
    db: DB,
    types: Annotated[list[schemas.EventType] | None, Query(description="Фильтр по типу, можно несколько")] = None,
    date_from: date | None = None,
    date_to: date | None = None,
):
    _plant(db, plant_id)
    wanted = set(types or ["water", "feed", "lamp", "repot"])
    return svc.history(db, plant_id, wanted, date_from, date_to)


@router.get("/{plant_id}/stats/weekly", response_model=list[schemas.WeekStatOut])
def plant_weekly_stats(plant_id: int, db: DB, weeks: Annotated[int, Query(ge=1, le=52)] = 8):
    _plant(db, plant_id)
    return svc.weekly(db, plant_id, weeks)
