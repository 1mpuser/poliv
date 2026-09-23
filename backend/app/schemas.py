from datetime import date, datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from app.models import FeedMethod, Season

Status = Literal["ok", "soon", "late", "off"]


class ORM(BaseModel):
    model_config = ConfigDict(from_attributes=True)


# ---------- Plant ----------
class PlantFields(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    species: str = Field("", max_length=100)
    location: str | None = Field(None, max_length=100)
    pot_size_l: float | None = Field(None, ge=0, le=1000)
    notes: str | None = None
    water_interval_days: int = Field(4, ge=1, le=90)
    fertilizing_enabled: bool = True
    lamp_hours_per_day: float = Field(12, ge=0, le=24)
    repot_check_interval_months: int = Field(12, ge=1, le=120)


class PlantCreate(PlantFields):
    pass


class PlantUpdate(BaseModel):
    name: str | None = Field(None, min_length=1, max_length=100)
    species: str | None = Field(None, max_length=100)
    location: str | None = Field(None, max_length=100)
    pot_size_l: float | None = Field(None, ge=0, le=1000)
    notes: str | None = None
    water_interval_days: int | None = Field(None, ge=1, le=90)
    fertilizing_enabled: bool | None = None
    lamp_hours_per_day: float | None = Field(None, ge=0, le=24)
    repot_check_interval_months: int | None = Field(None, ge=1, le=120)


class PlantOut(ORM, PlantFields):
    id: int
    added_at: datetime


# ---------- FertilizerType ----------
class FertilizerFields(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    npk: str = Field("", max_length=50)
    root_dose_ml_per_l: float | None = Field(None, ge=0)
    foliar_dose_ml_per_l: float | None = Field(None, ge=0)
    interval_days_active_season: int = Field(ge=1, le=365)
    interval_days_dormant_season: int | None = Field(None, ge=1, le=365)


class FertilizerCreate(FertilizerFields):
    pass


class FertilizerUpdate(BaseModel):
    name: str | None = Field(None, min_length=1, max_length=100)
    npk: str | None = Field(None, max_length=50)
    root_dose_ml_per_l: float | None = Field(None, ge=0)
    foliar_dose_ml_per_l: float | None = Field(None, ge=0)
    interval_days_active_season: int | None = Field(None, ge=1, le=365)
    interval_days_dormant_season: int | None = Field(None, ge=1, le=365)


class FertilizerOut(ORM, FertilizerFields):
    id: int


# ---------- Логи ----------
class WateringCreate(BaseModel):
    plant_id: int
    watered_at: datetime | None = None
    note: str | None = None


class WateringUpdate(BaseModel):
    watered_at: datetime | None = None
    note: str | None = None


class WateringOut(ORM):
    id: int
    plant_id: int
    watered_at: datetime
    note: str | None


class FeedingCreate(BaseModel):
    plant_id: int
    fertilizer_type_id: int
    method: FeedMethod
    fed_at: datetime | None = None
    note: str | None = None


class FeedingUpdate(BaseModel):
    fertilizer_type_id: int | None = None
    method: FeedMethod | None = None
    fed_at: datetime | None = None
    note: str | None = None


class FeedingOut(ORM):
    id: int
    plant_id: int
    fertilizer_type_id: int | None
    method: FeedMethod
    fed_at: datetime
    note: str | None


class LampSessionCreate(BaseModel):
    plant_id: int | None = None
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionUpdate(BaseModel):
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionOut(ORM):
    id: int
    plant_id: int | None
    started_at: datetime
    ended_at: datetime | None
    planned_hours_per_day: float


class LampToggle(BaseModel):
    plant_id: int | None = None


class LampToggleOut(BaseModel):
    is_on: bool
    session: LampSessionOut


class RepottingCreate(BaseModel):
    plant_id: int
    repotted_at: datetime | None = None
    pot_size_before: float | None = Field(None, ge=0)
    pot_size_after: float | None = Field(None, ge=0)
    note: str | None = None


class RepottingUpdate(BaseModel):
    repotted_at: datetime | None = None
    pot_size_before: float | None = Field(None, ge=0)
    pot_size_after: float | None = Field(None, ge=0)
    note: str | None = None


class RepottingOut(ORM):
    id: int
    plant_id: int
    repotted_at: datetime
    pot_size_before: float | None
    pot_size_after: float | None
    note: str | None


# ---------- Settings ----------
class SettingsOut(ORM):
    current_season: Season
    notify_days_ahead: int


class SettingsUpdate(BaseModel):
    current_season: Season | None = None
    notify_days_ahead: int | None = Field(None, ge=0, le=14)


# ---------- Сводка ----------
class WaterSummary(BaseModel):
    last_at: datetime | None
    days_since: int | None
    interval_days: int
    due_in_days: int
    status: Status


class FeedSummary(BaseModel):
    enabled: bool
    last_at: datetime | None
    last_fertilizer_name: str | None
    days_since: int | None
    next: FertilizerOut | None
    interval_days: int | None
    due_date: date | None
    due_in_days: int | None
    status: Status


class LampSummary(BaseModel):
    hours_today: float
    planned_hours: float
    status: Status
    is_on: bool
    open_session_id: int | None
    shared_is_on: bool


class RepotSummary(BaseModel):
    last_at: datetime | None
    interval_months: int
    next_check_date: date
    due_in_days: int
    status: Status


class PlantSummary(BaseModel):
    plant: PlantOut
    season: Season
    water: WaterSummary
    feed: FeedSummary
    lamp: LampSummary
    repot: RepotSummary


# ---------- История и статистика ----------
EventType = Literal["water", "feed", "lamp", "repot"]


class HistoryEvent(BaseModel):
    type: EventType
    id: int
    at: datetime
    note: str | None = None
    # feed
    fertilizer_name: str | None = None
    method: FeedMethod | None = None
    # lamp
    ended_at: datetime | None = None
    hours: float | None = None
    shared: bool | None = None
    # repot
    pot_size_before: float | None = None
    pot_size_after: float | None = None


class WeekStatOut(BaseModel):
    week_start: date
    waterings: int
    feedings: int
    lamp_hours: float
    is_current: bool


class Token(BaseModel):
    access_token: str
    token_type: str = "bearer"
