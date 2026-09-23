import enum
from datetime import datetime

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    Enum,
    Float,
    ForeignKey,
    Integer,
    String,
    Text,
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


class Plant(Base):
    __tablename__ = "plants"

    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str] = mapped_column(String(100))
    species: Mapped[str] = mapped_column(String(100), default="")
    location: Mapped[str | None] = mapped_column(String(100))
    pot_size_l: Mapped[float | None] = mapped_column(Float)
    added_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    notes: Mapped[str | None] = mapped_column(Text)

    # Настройки ухода per-plant
    water_interval_days: Mapped[int] = mapped_column(Integer, default=4)
    fertilizing_enabled: Mapped[bool] = mapped_column(Boolean, default=True)
    lamp_hours_per_day: Mapped[float] = mapped_column(Float, default=12)
    repot_check_interval_months: Mapped[int] = mapped_column(Integer, default=12)

    __table_args__ = (
        CheckConstraint("water_interval_days > 0", name="water_interval_positive"),
        CheckConstraint("lamp_hours_per_day BETWEEN 0 AND 24", name="lamp_hours_range"),
        CheckConstraint("repot_check_interval_months > 0", name="repot_interval_positive"),
    )


class FertilizerType(Base):
    __tablename__ = "fertilizer_types"

    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str] = mapped_column(String(100), unique=True)
    npk: Mapped[str] = mapped_column(String(50), default="")
    root_dose_ml_per_l: Mapped[float | None] = mapped_column(Float)
    foliar_dose_ml_per_l: Mapped[float | None] = mapped_column(Float)
    interval_days_active_season: Mapped[int] = mapped_column(Integer)
    # NULL — в спящий сезон этим удобрением не подкармливают
    interval_days_dormant_season: Mapped[int | None] = mapped_column(Integer)


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
    # NULL — общая лампа, светит на все растения
    plant_id: Mapped[int | None] = mapped_column(
        ForeignKey("plants.id", ondelete="CASCADE"), index=True
    )
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    # NULL — лампа горит сейчас
    ended_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    planned_hours_per_day: Mapped[float] = mapped_column(Float, default=12)

    __table_args__ = (
        CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="lamp_end_after_start"),
    )


class RepottingLog(Base):
    __tablename__ = "repotting_logs"

    id: Mapped[int] = mapped_column(primary_key=True)
    plant_id: Mapped[int] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    repotted_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    pot_size_before: Mapped[float | None] = mapped_column(Float)
    pot_size_after: Mapped[float | None] = mapped_column(Float)
    note: Mapped[str | None] = mapped_column(Text)


class AppSettings(Base):
    """Глобальные настройки — одна строка с id=1."""

    __tablename__ = "app_settings"

    id: Mapped[int] = mapped_column(primary_key=True, default=1)
    current_season: Mapped[Season] = mapped_column(_enum(Season, "season"), default=Season.active)
    # За сколько дней до срока статус становится «скоро»
    notify_days_ahead: Mapped[int] = mapped_column(Integer, default=1)

    __table_args__ = (CheckConstraint("id = 1", name="single_row"),)
