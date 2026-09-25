"""Лампы: растения под лампой (периоды привязки), сессии растения, переключение,
досветка до нормы и розетка в Умном доме Яндекса. Правила — чистые функции в summary.py."""

import logging
from collections import defaultdict
from collections.abc import Collection
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone

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


# ---------- переключение кнопкой ----------
@dataclass(frozen=True)
class Toggled:
    is_on: bool
    session: LampSession
    previous_ended_at: datetime | None = None


def toggle(db: Session, lamp: Lamp, now: datetime) -> Toggled:
    """Гасит то, что горит сейчас (вручную, по расписанию или досветка), иначе включает вручную.
    previous_ended_at нужен для отмены выключения. Розетка переключается сразу."""
    current = covering_session(db, lamp.id, now)
    if current is not None:
        result = Toggled(False, current, current.ended_at)
        current.ended_at = now
    else:
        current = LampSession(user_id=lamp.user_id, lamp_id=lamp.id, started_at=now, source=LampSource.manual)
        db.add(current)
        result = Toggled(True, current)
    db.commit()
    db.refresh(current)
    sync_plug(db, lamp, now)
    return result


# ---------- досветка до нормы ----------
def _today_bounds(now: datetime) -> tuple[datetime, datetime]:
    tz = settings.zone
    day = rules.local_date(now, tz)
    return rules.local_midnight(day, tz), rules.local_midnight(day + timedelta(days=1), tz)


def _auto_today(db: Session, lamp: Lamp, now: datetime) -> list[LampSession]:
    start, end = _today_bounds(now)
    return list(
        db.scalars(
            select(LampSession)
            .where(
                LampSession.lamp_id == lamp.id,
                LampSession.source == LampSource.auto,
                LampSession.started_at >= start,
                LampSession.started_at < end,
            )
            .order_by(LampSession.started_at)
        )
    )


def _worst_deficit(db: Session, ids: list[int], natural: float, now: datetime) -> float:
    """Нехватка самого требовательного растения лампы по плану дня (всё, что уже записано на сегодня)."""
    tz = settings.zone
    day = rules.local_date(now, tz)
    since = rules.local_midnight(day, tz)
    deficits = []
    for plant in db.scalars(select(Plant).where(Plant.id.in_(ids))):
        hours = rules.lamp_hours_in_day(as_rules(plant_sessions(db, plant.id, since)), day, now, tz, plan=True)
        deficits.append(rules.light_state(plant.light_target_hours, natural, hours).deficit_hours)
    return rules.lamp_need(deficits)


def _add_auto(db: Session, lamp: Lamp, window: tuple[datetime, datetime] | None, now: datetime) -> None:
    if window is None:
        return
    start, end = max(window[0], now), window[1]  # часть, которая уже должна идти, — с этой минуты
    if end > start:
        db.add(LampSession(user_id=lamp.user_id, lamp_id=lamp.id, source=LampSource.auto, started_at=start, ended_at=end))
        db.flush()


def replan_auto(db: Session, lamp: Lamp, now: datetime) -> None:
    """Досветка на сегодня: не начавшиеся части (утро до рассвета, вечер после заката, день между ними)
    пересоздаются по свежим данным; начавшиеся и прошедшие не трогаются — в том числе выключенные кнопкой.
    Части определяются по солнцу: утро — до рассвета, вечер — с заката, день — между ними (в пасмурный день
    остаток, который не влез в вечер, добирается днём вплотную перед закатом)."""
    tz = settings.zone
    day = rules.local_date(now, tz)
    autos = _auto_today(db, lamp, now)
    for s in autos:
        if s.started_at > now:
            db.delete(s)
    db.flush()
    started = [s for s in autos if s.started_at <= now]
    daylight = db.get(DaylightDay, (lamp.user_id, day))
    ids = plant_ids(db, lamp.id)
    if daylight is not None and ids:
        natural = daylight.sunshine_hours
        sunrise, sunset = daylight.sunrise, daylight.sunset
        if sunrise is not None and not any(s.started_at < sunrise for s in started):
            need = _worst_deficit(db, ids, natural, now)
            _add_auto(db, lamp, rules.plan_morning(need, sunrise, lamp.morning_not_before, day, tz), now)
        if sunset is not None and not any(s.started_at >= sunset for s in started):
            remaining = _worst_deficit(db, ids, natural, now)  # утро уже учтено
            _add_auto(db, lamp, rules.plan_evening(remaining, sunset, lamp.evening_not_after, day, tz), now)
        if (
            sunrise is not None
            and sunset is not None
            and not any(sunrise <= s.started_at < sunset for s in started)
        ):
            remaining = _worst_deficit(db, ids, natural, now)  # утро и вечер уже учтены
            _add_auto(db, lamp, rules.plan_day(remaining, sunrise, sunset), now)
    db.commit()


def stop_auto(db: Session, lamp: Lamp, now: datetime) -> None:
    """Режим больше не «Авто»: идущая досветка гаснет сейчас, будущая удаляется. Без commit."""
    for s in _auto_today(db, lamp, now):
        if s.started_at > now:
            db.delete(s)
        elif s.ended_at is None or s.ended_at > now:
            s.ended_at = now
    db.flush()


# ---------- розетка ----------
def yandex_token(db: Session, user_id: int) -> str | None:
    row = db.get(UserSettings, user_id)
    if row is None or row.yandex_token is None:
        return None
    return secret_box.decrypt(row.yandex_token, settings.jwt_secret)


def _fail(lamp: Lamp, message: str, now: datetime) -> None:
    lamp.last_error, lamp.last_error_at = message, now


def sync_plug(db: Session, lamp: Lamp, now: datetime) -> None:
    """Довести розетку до нужного состояния. Команда уходит только при смене (last_state) — ручное
    выключение через Алису Поливалка не перебивает до следующего перехода по плану.
    Ошибка записывается в лампу, last_state не меняется — повтор на следующем шаге."""
    if lamp.device_id is None:
        return
    want = lamp.archived_at is None and covering_session(db, lamp.id, now) is not None
    if lamp.last_state == want:
        return
    row = db.get(UserSettings, lamp.user_id)
    token = yandex_token(db, lamp.user_id)
    if token is None:
        _fail(lamp, "Не задан токен Яндекса", now)
    elif row.yandex_token_invalid:
        _fail(lamp, "Токен Яндекса недействителен — обновите его в настройках", now)
    else:
        try:
            yandex.set_on(token, lamp.device_id, want)
        except yandex.YandexAuthError as e:
            row.yandex_token_invalid = True
            _fail(lamp, str(e), now)
        except yandex.YandexError as e:
            _fail(lamp, str(e), now)
        else:
            lamp.last_state, lamp.last_error, lamp.last_error_at = want, None, None
    db.commit()


# ---------- изменения лампы ----------
def update(db: Session, lamp: Lamp, data: dict, ids: list[int] | None, now: datetime) -> None:
    """PATCH лампы (поля уже проверены). Смена режима убирает хвосты старого режима."""
    old_mode, old_device = lamp.mode, lamp.device_id
    for key, value in data.items():
        setattr(lamp, key, value)
    if lamp.device_id != old_device:
        lamp.last_state, lamp.last_error, lamp.last_error_at = None, None, None
    if ids is not None:
        set_plants(db, lamp, ids, now)
    if old_mode == LampMode.auto and lamp.mode != LampMode.auto:
        stop_auto(db, lamp, now)
    db.commit()
    if old_mode == LampMode.schedule and lamp.mode != LampMode.schedule:
        light_svc.replace_schedule(db, lamp.user_id, lamp.id, [])
    after_change(db, lamp, now)


def after_change(db: Session, lamp: Lamp, now: datetime) -> None:
    if lamp.mode == LampMode.auto:
        replan_auto(db, lamp, now)
    sync_plug(db, lamp, now)


def move_plant(db: Session, plant_id: int, lamp_id: int | None, now: datetime) -> None:
    """Перенести растение; досветка старой и новой лампы пересчитывается сразу."""
    old = current_lamp_id(db, plant_id)
    assign(db, plant_id, lamp_id, now)
    db.commit()
    for lid in {old, lamp_id} - {None}:
        lamp = db.get(Lamp, lid)
        if lamp.archived_at is None:
            after_change(db, lamp, now)


def archive(db: Session, lamp: Lamp, now: datetime) -> None:
    """«Удалить» лампу: растения без лампы, горящее гаснет, будущее и расписание удаляются,
    розетке — «выкл». Прошлые сессии остаются — история растений не меняется."""
    lamp.archived_at = now
    for pid in plant_ids(db, lamp.id):
        assign(db, pid, None, now)
    for s in db.scalars(
        select(LampSession).where(
            LampSession.lamp_id == lamp.id, or_(LampSession.ended_at.is_(None), LampSession.ended_at > now)
        )
    ):
        if s.started_at > now:
            db.delete(s)
        else:
            s.ended_at = now
    db.execute(delete(LampSchedule).where(LampSchedule.lamp_id == lamp.id))
    db.commit()
    sync_plug(db, lamp, now)


# ---------- ответы API ----------
def _planned(db: Session, lamp: Lamp, now: datetime) -> list[schemas.PlannedInterval]:
    return [schemas.PlannedInterval(start=s.started_at, end=s.ended_at) for s in _auto_today(db, lamp, now)]


def lamp_out(db: Session, lamp: Lamp, now: datetime) -> schemas.LampOut:
    return schemas.LampOut(
        id=lamp.id,
        name=lamp.name,
        mode=lamp.mode,
        device_id=lamp.device_id,
        device_name=lamp.device_name,
        morning_not_before=lamp.morning_not_before,
        evening_not_after=lamp.evening_not_after,
        last_state=lamp.last_state,
        last_error=lamp.last_error,
        last_error_at=lamp.last_error_at,
        plant_ids=plant_ids(db, lamp.id),
        is_on=covering_session(db, lamp.id, now) is not None,
        schedule=[
            schemas.ScheduleInterval(start_time=s.start_time, end_time=s.end_time)
            for s in light_svc.schedules_for(db, lamp.id)
        ],
        planned=_planned(db, lamp, now),
    )


def brief(db: Session, lamp: Lamp, now: datetime) -> schemas.LampBrief:
    return schemas.LampBrief(
        id=lamp.id,
        name=lamp.name,
        mode=lamp.mode,
        is_on=covering_session(db, lamp.id, now) is not None,
        has_device=lamp.device_id is not None,
        planned=_planned(db, lamp, now),
        last_error=lamp.last_error,
    )


# ---------- шаг раз в минуту ----------
def tick(db: Session, now: datetime | None = None) -> None:
    """Сессии по расписаниям на сегодня, досветка и розетки — для всех активных учёток."""
    now = now or datetime.now(timezone.utc)
    today = rules.local_date(now, settings.zone)
    for user_id in db.scalars(select(User.id).where(User.blocked_at.is_(None))).all():
        try:
            light_svc.ensure_schedule_sessions(db, user_id, today)
        except Exception:
            db.rollback()
            log.exception("schedule sessions failed for user %s", user_id)
    lamps = db.scalars(
        select(Lamp)
        .join(User, User.id == Lamp.user_id)
        .where(Lamp.archived_at.is_(None), User.blocked_at.is_(None))
        .order_by(Lamp.id)
    ).all()
    for lamp in lamps:
        try:
            if lamp.mode == LampMode.auto:
                replan_auto(db, lamp, now)
            sync_plug(db, lamp, now)
        except Exception:  # сеть, данные одной лампы — не мешаем остальным
            db.rollback()
            log.exception("lamp %s tick failed", lamp.id)
