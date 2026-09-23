import enum
from datetime import date, datetime, time

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    Date,
    DateTime,
    Enum,
    Float,
    ForeignKey,
    Integer,
    String,
    Text,
    Time,
    UniqueConstraint,
    func,
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


class Season(str, enum.Enum):
    active = "active"
    dormant = "dormant"


class FeedMethod(str, enum.Enum):
    root = "root"
    foliar = "foliar"


def _enum(cls: type[enum.Enum], name: str) -> Enum:
    return Enum(cls, name=name, values_callable=lambda e: [m.value for m in e])


class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(primary_key=True)
    email: Mapped[str] = mapped_column(String(254), unique=True)
    password_hash: Mapped[str] = mapped_column(String(255))
    is_admin: Mapped[bool] = mapped_column(Boolean, default=False)
    blocked_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    # Растёт при смене пароля и блокировке — выданные раньше JWT перестают действовать
    token_version: Mapped[int] = mapped_column(Integer, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


def _owner() -> Mapped[int]:
    return mapped_column(ForeignKey("users.id", ondelete="CASCADE"), index=True)


class Plant(Base):
    __tablename__ = "plants"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = _owner()
    name: Mapped[str] = mapped_column(String(100))
    species: Mapped[str] = mapped_column(String(100), default="")
    location: Mapped[str | None] = mapped_column(String(100))
    pot_size_l: Mapped[float | None] = mapped_column(Float)
    added_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    notes: Mapped[str | None] = mapped_column(Text)

    # Настройки ухода per-plant
    water_interval_days: Mapped[int] = mapped_column(Integer, default=4)
    fertilizing_enabled: Mapped[bool] = mapped_column(Boolean, default=True)
    # Норма всего света в день: солнечные часы + лампа
    light_target_hours: Mapped[float] = mapped_column(Float, default=12)
    repot_check_interval_months: Mapped[int] = mapped_column(Integer, default=12)

    __table_args__ = (
        CheckConstraint("water_interval_days > 0", name="water_interval_positive"),
        CheckConstraint("light_target_hours BETWEEN 0 AND 24", name="light_target_range"),
        CheckConstraint("repot_check_interval_months > 0", name="repot_interval_positive"),
    )


class FertilizerType(Base):
    __tablename__ = "fertilizer_types"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = _owner()
    name: Mapped[str] = mapped_column(String(100))
    npk: Mapped[str] = mapped_column(String(50), default="")
    root_dose_ml_per_l: Mapped[float | None] = mapped_column(Float)
    foliar_dose_ml_per_l: Mapped[float | None] = mapped_column(Float)
    interval_days_active_season: Mapped[int] = mapped_column(Integer)
    # NULL — в спящий сезон этим удобрением не подкармливают
    interval_days_dormant_season: Mapped[int | None] = mapped_column(Integer)

    __table_args__ = (UniqueConstraint("user_id", "name", name="uq_fertilizer_user_name"),)


class WateringLog(Base):
    __tablename__ = "watering_logs"

    id: Mapped[int] = mapped_column(primary_key=True)
    plant_id: Mapped[int] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    watered_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    note: Mapped[str | None] = mapped_column(Text)


class FeedingLog(Base):
    __tablename__ = "feeding_logs"

    id: Mapped[int] = mapped_column(primary_key=True)
    plant_id: Mapped[int] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    # SET NULL: удаление типа удобрения не стирает историю подкормок
    fertilizer_type_id: Mapped[int | None] = mapped_column(
        ForeignKey("fertilizer_types.id", ondelete="SET NULL")
    )
    method: Mapped[FeedMethod] = mapped_column(_enum(FeedMethod, "feed_method"))
    fed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    note: Mapped[str | None] = mapped_column(Text)

    fertilizer: Mapped[FertilizerType | None] = relationship(lazy="joined")


class LampSession(Base):
    __tablename__ = "lamp_sessions"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = _owner()
    # NULL — общая лампа, светит на все растения учётки
    plant_id: Mapped[int | None] = mapped_column(
        ForeignKey("plants.id", ondelete="CASCADE"), index=True
    )
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    # NULL — лампа горит сейчас
    ended_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    planned_hours_per_day: Mapped[float] = mapped_column(Float, default=12)
    # Не NULL — сессия создана по расписанию программируемой розетки
    schedule_id: Mapped[int | None] = mapped_column(
        ForeignKey("lamp_schedules.id", ondelete="SET NULL"), index=True
    )

    __table_args__ = (
        CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="lamp_end_after_start"),
    )


class LampSchedule(Base):
    """Интервал программируемой розетки (местное время). plant_id NULL — общая лампа учётки."""

    __tablename__ = "lamp_schedules"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = _owner()
    plant_id: Mapped[int | None] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    start_time: Mapped[time] = mapped_column(Time)
    end_time: Mapped[time] = mapped_column(Time)

    __table_args__ = (CheckConstraint("end_time > start_time", name="schedule_end_after_start"),)


class DaylightDay(Base):
    """Свет за день в городе учётки (Open-Meteo): солнечные часы учитывают облачность."""

    __tablename__ = "daylight_days"

    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), primary_key=True)
    day: Mapped[date] = mapped_column(Date, primary_key=True)
    sunrise: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    sunset: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    daylight_hours: Mapped[float] = mapped_column(Float)
    sunshine_hours: Mapped[float] = mapped_column(Float)
    fetched_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class RepottingLog(Base):
    __tablename__ = "repotting_logs"

    id: Mapped[int] = mapped_column(primary_key=True)
    plant_id: Mapped[int] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    repotted_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    pot_size_before: Mapped[float | None] = mapped_column(Float)
    pot_size_after: Mapped[float | None] = mapped_column(Float)
    note: Mapped[str | None] = mapped_column(Text)


class UserSettings(Base):
    """Настройки учётки: сезон и порог «скоро»."""

    __tablename__ = "user_settings"

    user_id: Mapped[int] = mapped_column(ForeignKey("users.id", ondelete="CASCADE"), primary_key=True)
    current_season: Mapped[Season] = mapped_column(_enum(Season, "season"), default=Season.active)
    # За сколько дней до срока статус становится «скоро»
    notify_days_ahead: Mapped[int] = mapped_column(Integer, default=1)
    # Город для светового дня
    location_name: Mapped[str | None] = mapped_column(String(200))
    latitude: Mapped[float | None] = mapped_column(Float)
    longitude: Mapped[float | None] = mapped_column(Float)
