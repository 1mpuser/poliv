"""Сборка сводки, истории и статистики растения из БД. Правила — в summary.py."""

from collections.abc import Sequence
from datetime import date, datetime, timedelta, timezone

from sqlalchemy import func, or_, select
from sqlalchemy.orm import Session

from app import schemas
from app.config import settings
from app.models import (
    DaylightDay,
    FeedingLog,
    FertilizerType,
    LampSession,
    Plant,
    RepottingLog,
    User,
    UserSettings,
    WateringLog,
)
from app.services import light as light_svc
from app.services import summary as rules


def now_utc() -> datetime:
    return datetime.now(timezone.utc)


def get_user_settings(db: Session, user_id: int) -> UserSettings:
    row = db.get(UserSettings, user_id)
    if row is None:  # учётка создана в обход create_user
        row = UserSettings(user_id=user_id)
        db.add(row)
        db.commit()
    return row


def _lamp_filter(plant: Plant):
    """Сессии, которые светят на растение: его собственные и общая лампа той же учётки."""
    return or_(
        LampSession.plant_id == plant.id,
        (LampSession.plant_id.is_(None)) & (LampSession.user_id == plant.user_id),
    )


def _lamp_sessions(db: Session, plant: Plant, since: datetime) -> list[rules.Session]:
    rows = db.execute(
        select(LampSession.started_at, LampSession.ended_at).where(
            _lamp_filter(plant),
            or_(LampSession.ended_at.is_(None), LampSession.ended_at >= since),
        )
    ).all()
    return [(s, e) for s, e in rows]


def covering_session(db: Session, user_id: int, plant_id: int | None, now: datetime) -> LampSession | None:
    """Сессия лампы растения (или общей при plant_id=None), которая горит прямо сейчас:
    включённая вручную или идущая по расписанию. Ручная — в приоритете."""
    cond = LampSession.plant_id.is_(None) if plant_id is None else LampSession.plant_id == plant_id
    return db.scalars(
        select(LampSession)
        .where(
            LampSession.user_id == user_id,
            cond,
            LampSession.started_at <= now,
            or_(LampSession.ended_at.is_(None), LampSession.ended_at > now),
        )
        .order_by(LampSession.ended_at.is_(None).desc(), LampSession.started_at.desc())
    ).first()


def _intervals(schedules) -> list[schemas.ScheduleInterval]:
    return [schemas.ScheduleInterval(start_time=s.start_time, end_time=s.end_time) for s in schedules]


def _light(
    db: Session, plant: Plant, app: UserSettings, daylight: DaylightDay | None,
    sessions: list[rules.Session], now: datetime,
) -> schemas.LightSummary:
    tz = settings.zone
    today = rules.local_date(now, tz)
    lamp = rules.lamp_hours_in_day(sessions, today, now, tz, plan=True)
    natural = daylight.sunshine_hours if daylight else None
    state = rules.light_state(plant.light_target_hours, natural, lamp)
    window = rules.suggest_lamp_window(
        state.deficit_hours, daylight.sunset if daylight else None,
        [e or now for _, e in sessions], today, tz,
    )
    if window and window[1] <= now:  # подсказка уже в прошлом — только дефицит
        window = None
    return schemas.LightSummary(
        target_hours=plant.light_target_hours,
        location_name=app.location_name,
        natural_hours=natural,
        daylight_hours=daylight.daylight_hours if daylight else None,
        sunrise=daylight.sunrise if daylight else None,
        sunset=daylight.sunset if daylight else None,
        lamp_hours=round(lamp, 2),
        total_hours=round(state.total_hours, 2),
        deficit_hours=round(state.deficit_hours, 2),
        status=state.status,
        suggestion_start=window[0] if window else None,
        suggestion_end=window[1] if window else None,
        suggestion_until_midnight=bool(window and window[2]),
        schedule=_intervals(light_svc.schedules_for(db, plant.user_id, plant.id)),
        shared_schedule=_intervals(light_svc.schedules_for(db, plant.user_id, None)),
    )


def build_summary(
    db: Session,
    plant: Plant,
    app: UserSettings,
    fertilizers: Sequence[FertilizerType],
    now: datetime,
    daylight: DaylightDay | None = None,
) -> schemas.PlantSummary:
    tz = settings.zone
    ahead = app.notify_days_ahead

    last_water = db.scalar(select(func.max(WateringLog.watered_at)).where(WateringLog.plant_id == plant.id))
    water = rules.water_state(last_water, plant.water_interval_days, ahead, now, tz)

    last_feed = db.scalars(
        select(FeedingLog).where(FeedingLog.plant_id == plant.id).order_by(FeedingLog.fed_at.desc()).limit(1)
    ).first()
    feed = rules.feed_state(
        plant.fertilizing_enabled,
        fertilizers,
        last_feed.fed_at if last_feed else None,
        last_feed.fertilizer_type_id if last_feed else None,
        app.current_season.value,
        ahead,
        now,
        tz,
    )

    midnight = rules.local_midnight(rules.local_date(now, tz), tz)
    sessions = _lamp_sessions(db, plant, midnight)
    hours = rules.lamp_hours_today(sessions, now, tz)
    own = covering_session(db, plant.user_id, plant.id, now)

    last_repot = db.scalar(
        select(func.max(RepottingLog.repotted_at)).where(RepottingLog.plant_id == plant.id)
    )
    repot = rules.repot_state(last_repot, plant.added_at, plant.repot_check_interval_months, now, tz)

    return schemas.PlantSummary(
        plant=schemas.PlantOut.model_validate(plant),
        season=app.current_season,
        water=schemas.WaterSummary(
            last_at=last_water,
            days_since=water.days_since,
            interval_days=plant.water_interval_days,
            due_in_days=water.due_in_days,
            status=water.status,
        ),
        feed=schemas.FeedSummary(
            enabled=plant.fertilizing_enabled,
            last_at=last_feed.fed_at if last_feed else None,
            last_fertilizer_name=last_feed.fertilizer.name if last_feed and last_feed.fertilizer else None,
            days_since=feed.days_since,
            next=schemas.FertilizerOut.model_validate(feed.next) if feed.next else None,
            interval_days=feed.interval_days,
            due_date=feed.due_date,
            due_in_days=feed.due_in_days,
            status=feed.status,
        ),
        lamp=schemas.LampSummary(
            hours_today=round(hours, 2),
            planned_hours=plant.light_target_hours,
            status=rules.lamp_status(hours, plant.light_target_hours),
            is_on=own is not None,
            open_session_id=own.id if own else None,
            shared_is_on=covering_session(db, plant.user_id, None, now) is not None,
        ),
        light=_light(db, plant, app, daylight, sessions, now),
        repot=schemas.RepotSummary(
            last_at=last_repot,
            interval_months=plant.repot_check_interval_months,
            next_check_date=repot.next_check_date,
            due_in_days=repot.due_in_days,
            status=repot.status,
        ),
    )


def build_summaries(db: Session, user: User, plants: Sequence[Plant]) -> list[schemas.PlantSummary]:
    app = get_user_settings(db, user.id)
    fertilizers = db.scalars(select(FertilizerType).where(FertilizerType.user_id == user.id)).all()
    now = now_utc()
    daylight = light_svc.today_daylight(db, user.id)
    return [build_summary(db, p, app, fertilizers, now, daylight) for p in plants]


def history(
    db: Session,
    plant: Plant,
    types: set[str],
    date_from: date | None,
    date_to: date | None,
) -> list[schemas.HistoryEvent]:
    tz = settings.zone
    start = rules.local_midnight(date_from, tz) if date_from else None
    end = rules.local_midnight(date_to + timedelta(days=1), tz) if date_to else None
    now = now_utc()
    plant_id = plant.id

    def in_range(col):
        conds = []
        if start is not None:
            conds.append(col >= start)
        if end is not None:
            conds.append(col < end)
        return conds

    events: list[schemas.HistoryEvent] = []
    if "water" in types:
        for w in db.scalars(
            select(WateringLog).where(WateringLog.plant_id == plant_id, *in_range(WateringLog.watered_at))
        ):
            events.append(schemas.HistoryEvent(type="water", id=w.id, at=w.watered_at, note=w.note))
    if "feed" in types:
        for f in db.scalars(
            select(FeedingLog).where(FeedingLog.plant_id == plant_id, *in_range(FeedingLog.fed_at))
        ):
            events.append(
                schemas.HistoryEvent(
                    type="feed",
                    id=f.id,
                    at=f.fed_at,
                    note=f.note,
                    fertilizer_name=f.fertilizer.name if f.fertilizer else None,
                    method=f.method,
                )
            )
    if "lamp" in types:
        for s in db.scalars(
            select(LampSession).where(_lamp_filter(plant), *in_range(LampSession.started_at))
        ):
            hours = ((s.ended_at or now) - s.started_at).total_seconds() / 3600
            events.append(
                schemas.HistoryEvent(
                    type="lamp",
                    id=s.id,
                    at=s.started_at,
                    ended_at=s.ended_at,
                    hours=round(hours, 2),
                    shared=s.plant_id is None,
                )
            )
    if "repot" in types:
        for r in db.scalars(
            select(RepottingLog).where(RepottingLog.plant_id == plant_id, *in_range(RepottingLog.repotted_at))
        ):
            events.append(
                schemas.HistoryEvent(
                    type="repot",
                    id=r.id,
                    at=r.repotted_at,
                    note=r.note,
                    pot_size_before=r.pot_size_before,
                    pot_size_after=r.pot_size_after,
                )
            )
    events.sort(key=lambda e: e.at, reverse=True)
    return events


def weekly(db: Session, plant: Plant, weeks: int) -> list[schemas.WeekStatOut]:
    plant_id = plant.id
    tz = settings.zone
    now = now_utc()
    today = rules.local_date(now, tz)
    since = rules.local_midnight(today - timedelta(days=today.weekday(), weeks=weeks - 1), tz)
    waterings = db.scalars(
        select(WateringLog.watered_at).where(WateringLog.plant_id == plant_id, WateringLog.watered_at >= since)
    ).all()
    feedings = db.scalars(
        select(FeedingLog.fed_at).where(FeedingLog.plant_id == plant_id, FeedingLog.fed_at >= since)
    ).all()
    sunshine = light_svc.sunshine_by_day(db, plant.user_id, since.date())
    # будущие дни прогноза в статистику не попадают
    sunshine = {d: h for d, h in sunshine.items() if d <= today}
    stats = rules.weekly_stats(
        waterings, feedings, _lamp_sessions(db, plant, since), weeks, now, tz, sunshine=sunshine
    )
    return [
        schemas.WeekStatOut(
            week_start=w.week_start,
            waterings=w.waterings,
            feedings=w.feedings,
            lamp_hours=round(w.lamp_hours, 1),
            sunshine_hours=round(w.sunshine_hours, 1),
            is_current=w.is_current,
        )
        for w in stats
    ]
