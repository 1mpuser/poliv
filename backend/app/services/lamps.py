"""Лампы: растения под лампой (периоды привязки), сессии растения, переключение,
досветка до нормы и розетка в Умном доме Яндекса. Правила — чистые функции в summary.py."""

import logging
from collections import defaultdict
from collections.abc import Collection
from dataclasses import dataclass
from datetime import datetime, time, timedelta, timezone

from sqlalchemy import delete, or_, select
from sqlalchemy.orm import Session

from app import schemas
from app.config import settings
from app.models import DaylightDay, Lamp, LampMode, LampSchedule, LampSession, LampSource, Plant, PlantLamp, User, UserSettings
from app.services import light as light_svc
from app.services import secret_box, yandex
from app.services import summary as rules

log = logging.getLogger("poliv.lamps")


# ---------- растения под лампой ----------
def current_lamp_id(db: Session, plant_id: int) -> int | None:
    return db.scalar(select(PlantLamp.lamp_id).where(PlantLamp.plant_id == plant_id, PlantLamp.ended_at.is_(None)))


def plant_ids(db: Session, lamp_id: int) -> list[int]:
    return list(
        db.scalars(
            select(PlantLamp.plant_id)
            .where(PlantLamp.lamp_id == lamp_id, PlantLamp.ended_at.is_(None))
            .order_by(PlantLamp.plant_id)
        )
    )


def assign(db: Session, plant_id: int, lamp_id: int | None, now: datetime) -> None:
    """Поставить растение под лампу (None — без лампы). Прошлые периоды не меняются. Без commit."""
    current = db.scalars(select(PlantLamp).where(PlantLamp.plant_id == plant_id, PlantLamp.ended_at.is_(None))).first()
    if current is not None and current.lamp_id == lamp_id:
        return
    if current is not None:
        current.ended_at = now
        db.flush()  # сначала закрыть: открытый период у растения один (uq_plant_lamp_open)
    if lamp_id is not None:
        db.add(PlantLamp(plant_id=plant_id, lamp_id=lamp_id, started_at=now))
        db.flush()


def set_plants(db: Session, lamp: Lamp, ids: Collection[int], now: datetime) -> None:
    """Полная замена растений лампы. Владелец растений проверен вызывающим. Без commit."""
    for pid in plant_ids(db, lamp.id):
        if pid not in ids:
            assign(db, pid, None, now)
    for pid in ids:
        assign(db, pid, lamp.id, now)


# ---------- сессии растения ----------
@dataclass(frozen=True)
class PlantSession:
    id: int
    lamp_id: int
    lamp_name: str
    started_at: datetime
    ended_at: datetime | None


def plant_sessions(db: Session, plant_id: int, since: datetime | None = None) -> list[PlantSession]:
    """Сессии ламп, под которыми растение стояло, обрезанные по периодам привязки."""
    periods: dict[int, list[rules.Period]] = defaultdict(list)
    for lamp_id, s, e in db.execute(
        select(PlantLamp.lamp_id, PlantLamp.started_at, PlantLamp.ended_at).where(PlantLamp.plant_id == plant_id)
    ):
        periods[lamp_id].append((s, e))
    if not periods:
        return []
    q = select(LampSession, Lamp.name).join(Lamp, Lamp.id == LampSession.lamp_id).where(LampSession.lamp_id.in_(periods))
    if since is not None:
        q = q.where(or_(LampSession.ended_at.is_(None), LampSession.ended_at >= since))
    out = []
    for sess, name in db.execute(q):
        for s, e in rules.clip_session(sess.started_at, sess.ended_at, periods[sess.lamp_id]):
            out.append(PlantSession(sess.id, sess.lamp_id, name, s, e))
    return out


def as_rules(sessions: list[PlantSession]) -> list[rules.Session]:
    return [(p.started_at, p.ended_at) for p in sessions]


def covering_session(db: Session, lamp_id: int, now: datetime) -> LampSession | None:
    """Сессия лампы, которая горит прямо сейчас (ручная, по расписанию или досветка). Ручная — в приоритете."""
    return db.scalars(
        select(LampSession)
        .where(
            LampSession.lamp_id == lamp_id,
            LampSession.started_at <= now,
            or_(LampSession.ended_at.is_(None), LampSession.ended_at > now),
        )
        .order_by(LampSession.ended_at.is_(None).desc(), LampSession.started_at.desc())
    ).first()
