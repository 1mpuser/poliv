"""Свет: световой день из Open-Meteo и сессии лампы по расписанию."""

import json
import logging
import urllib.parse
import urllib.request
from datetime import date, datetime, time, timedelta

from fastapi import HTTPException, status
from sqlalchemy import delete, select
from sqlalchemy.orm import Session

from app import schemas
from app.config import settings
from app.models import DaylightDay, Lamp, LampMode, LampSchedule, LampSession, LampSource, User, UserSettings
from app.services import summary as rules

log = logging.getLogger("poliv.light")

GEOCODE_URL = "https://geocoding-api.open-meteo.com/v1/search"
FORECAST_URL = "https://api.open-meteo.com/v1/forecast"
TIMEOUT = 10
# Прогноз солнечных часов на сегодня уточняется в течение дня — перезапрашиваем не чаще раза в 3 часа
REFRESH_AFTER = timedelta(hours=3)


def _get_json(url: str, params: dict) -> dict:
    req = urllib.request.Request(f"{url}?{urllib.parse.urlencode(params)}", headers={"User-Agent": "poliv/1.0"})
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.load(resp)


def geocode(query: str) -> list[dict]:
    data = _get_json(GEOCODE_URL, {"name": query, "count": 8, "language": "ru", "format": "json"})
    return [
        {
            "name": r["name"],
            "region": r.get("admin1"),
            "country": r.get("country"),
            "latitude": r["latitude"],
            "longitude": r["longitude"],
        }
        for r in data.get("results", [])
    ]


def fetch_days(latitude: float, longitude: float) -> list[dict]:
    """Позавчера … послезавтра: восход, закат, световой день и солнечные часы (часы, местное время)."""
    data = _get_json(
        FORECAST_URL,
        {
            "latitude": latitude,
            "longitude": longitude,
            "daily": "sunrise,sunset,daylight_duration,sunshine_duration",
            "timezone": settings.tz,
            "past_days": 2,
            "forecast_days": 3,
        },
    )
    d = data["daily"]
    tz = settings.zone

    def at(v: str | None) -> datetime | None:
        return datetime.fromisoformat(v).replace(tzinfo=tz) if v else None

    return [
        {
            "day": date.fromisoformat(d["time"][i]),
            "sunrise": at(d["sunrise"][i]),
            "sunset": at(d["sunset"][i]),
            "daylight_hours": round((d["daylight_duration"][i] or 0) / 3600, 2),
            "sunshine_hours": round((d["sunshine_duration"][i] or 0) / 3600, 2),
        }
        for i in range(len(d["time"]))
    ]


def sync_daylight(db: Session, user_settings: UserSettings, force: bool = False) -> bool:
    """Обновить свет по городу учётки. Возвращает True, если ходили в Open-Meteo."""
    if user_settings.latitude is None or user_settings.longitude is None:
        return False
    now = datetime.now(settings.zone)
    today = db.get(DaylightDay, (user_settings.user_id, now.date()))
    if not force and today is not None and now - today.fetched_at < REFRESH_AFTER:
        return False
    for row in fetch_days(user_settings.latitude, user_settings.longitude):
        db.merge(DaylightDay(user_id=user_settings.user_id, fetched_at=now, **row))
    db.commit()
    return True


def today_daylight(db: Session, user_id: int) -> DaylightDay | None:
    return db.get(DaylightDay, (user_id, rules.local_date(datetime.now(settings.zone), settings.zone)))


def sunshine_by_day(db: Session, user_id: int, since: date) -> dict[date, float]:
    rows = db.execute(
        select(DaylightDay.day, DaylightDay.sunshine_hours).where(
            DaylightDay.user_id == user_id, DaylightDay.day >= since
        )
    ).all()
    return {d: h for d, h in rows}


# ---------- расписание → сессии ----------
def schedules_for(db: Session, lamp_id: int) -> list[LampSchedule]:
    return list(
        db.scalars(select(LampSchedule).where(LampSchedule.lamp_id == lamp_id).order_by(LampSchedule.start_time))
    )


def validate_intervals(intervals: list[schemas.ScheduleInterval]) -> list[tuple[time, time]]:
    out = sorted((i.start_time, i.end_time) for i in intervals)
    for s, e in out:
        if e <= s:
            raise HTTPException(status.HTTP_400_BAD_REQUEST, "Конец интервала должен быть позже начала (через полночь — двумя интервалами)")
    for (_, e1), (s2, _) in zip(out, out[1:]):
        if s2 < e1:
            raise HTTPException(status.HTTP_400_BAD_REQUEST, "Интервалы пересекаются")
    return out


def ensure_schedule_sessions(db: Session, user_id: int, day: date) -> int:
    """Создать на день сессии по расписаниям ламп в режиме «по расписанию» (идемпотентно)."""
    tz = settings.zone
    start, end = rules.local_midnight(day, tz), rules.local_midnight(day + timedelta(days=1), tz)
    created = 0
    q = (
        select(LampSchedule)
        .join(Lamp, Lamp.id == LampSchedule.lamp_id)
        .where(LampSchedule.user_id == user_id, Lamp.mode == LampMode.schedule, Lamp.archived_at.is_(None))
    )
    for sch in db.scalars(q):
        exists = db.scalar(
            select(LampSession.id).where(
                LampSession.schedule_id == sch.id, LampSession.started_at >= start, LampSession.started_at < end
            )
        )
        if exists:
            continue
        (s, e), = rules.schedule_sessions_for_day([(sch.start_time, sch.end_time)], day, tz)
        db.add(
            LampSession(
                user_id=user_id, lamp_id=sch.lamp_id, schedule_id=sch.id,
                source=LampSource.schedule, started_at=s, ended_at=e,
            )
        )
        created += 1
    db.commit()
    return created


def replace_schedule(
    db: Session, user_id: int, lamp_id: int, intervals: list[tuple[time, time]]
) -> list[LampSchedule]:
    """Новое расписание лампы. Сегодняшние сессии по старому расписанию пересоздаются,
    прошлые дни не трогаются (история остаётся как была)."""
    tz = settings.zone
    today = datetime.now(tz).date()
    old_ids = [s.id for s in schedules_for(db, lamp_id)]
    if old_ids:
        db.execute(
            delete(LampSession).where(
                LampSession.schedule_id.in_(old_ids),
                LampSession.started_at >= rules.local_midnight(today, tz),
            )
        )
        db.execute(delete(LampSchedule).where(LampSchedule.id.in_(old_ids)))
    for s, e in intervals:
        db.add(LampSchedule(user_id=user_id, lamp_id=lamp_id, start_time=s, end_time=e))
    db.commit()
    ensure_schedule_sessions(db, user_id, today)
    return schedules_for(db, lamp_id)


# ---------- фоновая синхронизация ----------
def sync_all(db: Session) -> None:
    """Раз в полчаса: свет по городу для всех учёток. Сессии по расписаниям — в шаге ламп (lamps.tick)."""
    for user in db.scalars(select(User).where(User.blocked_at.is_(None))):
        try:
            row = db.get(UserSettings, user.id)
            if row is not None:
                sync_daylight(db, row)
        except Exception:  # сеть, Open-Meteo — не мешаем остальным учёткам
            db.rollback()
            log.exception("sync failed for user %s", user.id)
