from datetime import date, datetime, time
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from app.models import FeedMethod, LampMode, LampSource, Season

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
    light_target_hours: float = Field(12, ge=0, le=24)
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
    light_target_hours: float | None = Field(None, ge=0, le=24)
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
    lamp_id: int
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionUpdate(BaseModel):
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionOut(ORM):
    id: int
    lamp_id: int
    source: LampSource
    started_at: datetime
    ended_at: datetime | None
    planned_hours_per_day: float


class LampToggle(BaseModel):
    plant_id: int


class LampToggleOut(BaseModel):
    is_on: bool
    session: LampSessionOut
    # Для отмены выключения: вернуть ended_at к этому значению (null — горела вручную)
    previous_ended_at: datetime | None = None
    # Сессия записана, но розетка не ответила — текст для тоста
    plug_error: str | None = None


class ScheduleInterval(BaseModel):
    start_time: time
    end_time: time


class LampScheduleSet(BaseModel):
    """Полная замена расписания лампы; пустой список — расписания нет."""

    intervals: list[ScheduleInterval] = Field(default_factory=list, max_length=8)


# ---------- Лампы ----------
class LampFields(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    mode: LampMode = LampMode.manual
    device_id: str | None = Field(None, max_length=100)
    device_name: str | None = Field(None, max_length=200)
    morning_not_before: time = time(6)
    evening_not_after: time = time(23)


class LampCreate(LampFields):
    plant_ids: list[int] = Field(default_factory=list, max_length=100)


class LampUpdate(BaseModel):
    name: str | None = Field(None, min_length=1, max_length=100)
    mode: LampMode | None = None
    device_id: str | None = Field(None, max_length=100)
    device_name: str | None = Field(None, max_length=200)
    morning_not_before: time | None = None
    evening_not_after: time | None = None
    plant_ids: list[int] | None = Field(None, max_length=100)


class PlannedInterval(BaseModel):
    start: datetime
    end: datetime | None


class LampOut(LampFields):
    id: int
    last_state: bool | None
    last_error: str | None
    last_error_at: datetime | None
    plant_ids: list[int]
    is_on: bool
    schedule: list[ScheduleInterval]
    planned: list[PlannedInterval]  # досветка на сегодня (режим auto)


class LampBrief(BaseModel):
    """Лампа растения в сводке."""

    id: int
    name: str
    mode: LampMode
    is_on: bool
    has_device: bool
    planned: list[PlannedInterval]
    last_error: str | None


class PlantLampSet(BaseModel):
    lamp_id: int | None


class YandexTokenSet(BaseModel):
    token: str | None = Field(None, min_length=10, max_length=2000)


class YandexDevice(BaseModel):
    id: str
    name: str
    room: str | None
    type: str


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
    location_name: str | None
    latitude: float | None
    longitude: float | None
    yandex_status: Literal["none", "ok", "invalid"]


class SettingsUpdate(BaseModel):
    current_season: Season | None = None
    notify_days_ahead: int | None = Field(None, ge=0, le=14)
    location_name: str | None = Field(None, max_length=200)
    latitude: float | None = Field(None, ge=-90, le=90)
    longitude: float | None = Field(None, ge=-180, le=180)


class Place(BaseModel):
    name: str
    region: str | None
    country: str | None
    latitude: float
    longitude: float


class DaylightOut(ORM):
    day: date
    sunrise: datetime | None
    sunset: datetime | None
    daylight_hours: float
    sunshine_hours: float


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


class LightSummary(BaseModel):
    """Свет сегодня: солнечные часы (с облачностью) + лампа по плану дня против нормы растения."""

    target_hours: float
    location_name: str | None
    natural_hours: float | None  # None — город не задан или данных ещё нет
    daylight_hours: float | None
    sunrise: datetime | None
    sunset: datetime | None
    lamp_hours: float  # план на весь день: расписание + ручные включения
    total_hours: float
    deficit_hours: float
    status: Status
    suggestion_start: datetime | None  # когда добрать недостающее
    suggestion_end: datetime | None
    suggestion_until_midnight: bool  # даже до полуночи не хватит
    lamp: LampBrief | None  # лампа растения сейчас


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
    light: LightSummary
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
    lamp_name: str | None = None
    # repot
    pot_size_before: float | None = None
    pot_size_after: float | None = None


class WeekStatOut(BaseModel):
    week_start: date
    waterings: int
    feedings: int
    lamp_hours: float
    sunshine_hours: float
    is_current: bool


class Token(BaseModel):
    access_token: str
    token_type: str = "bearer"
