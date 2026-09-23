"""Правила статусов ухода. Чистые функции без БД — всё время приходит аргументами.

Дни считаются календарными в локальном часовом поясе: полив вчера в 23:30
и сегодня в 00:10 — это уже «1 день назад».
"""

import calendar
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import date, datetime, time, timedelta
from typing import Literal, Protocol, TypeVar
from zoneinfo import ZoneInfo

Status = Literal["ok", "soon", "late", "off"]
Session = tuple[datetime, datetime | None]

# Проверка пересадки планируется на месяцы вперёд — предупреждаем за две недели
REPOT_SOON_DAYS = 14


def status_for(due_in_days: int, ahead: int) -> Status:
    if due_in_days <= 0:
        return "late"
    if due_in_days <= ahead:
        return "soon"
    return "ok"


def local_date(dt: datetime, tz: ZoneInfo) -> date:
    return dt.astimezone(tz).date()


def local_midnight(d: date, tz: ZoneInfo) -> datetime:
    return datetime.combine(d, time.min, tzinfo=tz)


# ---------- полив ----------
@dataclass(frozen=True)
class WaterState:
    days_since: int | None
    due_in_days: int
    status: Status


def water_state(
    last_watered: datetime | None, interval_days: int, ahead: int, now: datetime, tz: ZoneInfo
) -> WaterState:
    if last_watered is None:
        return WaterState(None, 0, "late")
    days_since = (local_date(now, tz) - local_date(last_watered, tz)).days
    due_in = interval_days - days_since
    return WaterState(days_since, due_in, status_for(due_in, ahead))


# ---------- подкормка ----------
class FertilizerLike(Protocol):
    id: int
    interval_days_active_season: int
    interval_days_dormant_season: int | None


F = TypeVar("F", bound=FertilizerLike)


def pick_next_fertilizer(types: Sequence[F], last_type_id: int | None) -> F | None:
    """Следующий тип по кругу (порядок — по id), не совпадающий с последним.

    Если последний тип удалён, берём первый с большим id.
    """
    ordered = sorted(types, key=lambda t: t.id)
    if not ordered:
        return None
    if last_type_id is None:
        return ordered[0]
    for t in ordered:
        if t.id > last_type_id:
            return t
    return ordered[0]


def season_interval(ft: FertilizerLike, season: str) -> int | None:
    if season == "dormant":
        return ft.interval_days_dormant_season
    return ft.interval_days_active_season


@dataclass(frozen=True)
class FeedState:
    next: FertilizerLike | None
    interval_days: int | None
    due_date: date | None
    due_in_days: int | None
    days_since: int | None
    status: Status


def feed_state(
    enabled: bool,
    types: Sequence[F],
    last_fed_at: datetime | None,
    last_type_id: int | None,
    season: str,
    ahead: int,
    now: datetime,
    tz: ZoneInfo,
) -> FeedState:
    today = local_date(now, tz)
    days_since = (today - local_date(last_fed_at, tz)).days if last_fed_at else None
    nxt = pick_next_fertilizer(types, last_type_id)
    interval = season_interval(nxt, season) if nxt else None
    if not enabled or nxt is None or not interval:
        return FeedState(nxt, interval, None, None, days_since, "off")
    due_date = local_date(last_fed_at, tz) + timedelta(days=interval) if last_fed_at else today
    due_in = (due_date - today).days
    return FeedState(nxt, interval, due_date, due_in, days_since, status_for(due_in, ahead))


# ---------- лампа ----------
def lamp_hours_between(
    sessions: Sequence[Session], start: datetime, end: datetime, now: datetime
) -> float:
    """Часы горения в окне [start, end). Пересекающиеся сессии (своя + общая лампа)
    объединяются, открытая сессия считается до now."""
    clipped = []
    for s, e in sessions:
        e = e or now
        s, e = max(s, start), min(e, end, now)
        if e > s:
            clipped.append((s, e))
    clipped.sort()
    total = timedelta()
    cur_s = cur_e = None
    for s, e in clipped:
        if cur_e is None or s > cur_e:
            if cur_e is not None:
                total += cur_e - cur_s
            cur_s, cur_e = s, e
        else:
            cur_e = max(cur_e, e)
    if cur_e is not None:
        total += cur_e - cur_s
    return total.total_seconds() / 3600


def lamp_hours_in_day(
    sessions: Sequence[Session], day: date, now: datetime, tz: ZoneInfo, plan: bool
) -> float:
    """Часы лампы за день. plan=True — план на весь день: запланированные (по расписанию) сессии
    считаются целиком, горящая вручную — до now. plan=False — только уже отгоревшее."""
    start, end = local_midnight(day, tz), local_midnight(day + timedelta(days=1), tz)
    if not plan:
        return lamp_hours_between(sessions, start, end, now)
    # открытые сессии закрываем на now, закрытые берём целиком (в т.ч. будущие)
    closed = [(s, e if e is not None else now) for s, e in sessions]
    return lamp_hours_between(closed, start, end, end)


def lamp_hours_today(sessions: Sequence[Session], now: datetime, tz: ZoneInfo) -> float:
    today = local_date(now, tz)
    return lamp_hours_between(
        sessions, local_midnight(today, tz), local_midnight(today + timedelta(days=1), tz), now
    )


def lamp_status(hours: float, planned: float) -> Status:
    """Зелёный — набрано ≥90% нормы, жёлтый — ≥50%, красный — меньше."""
    if planned <= 0:
        return "ok"
    ratio = hours / planned
    if ratio >= 0.9:
        return "ok"
    if ratio >= 0.5:
        return "soon"
    return "late"


# ---------- свет: естественный + лампа против нормы ----------
@dataclass(frozen=True)
class LightState:
    total_hours: float
    deficit_hours: float
    status: Status


def light_state(target: float, natural: float | None, lamp: float) -> LightState:
    """natural — солнечные часы за день (None: город не задан — считаем только лампу)."""
    total = (natural or 0) + lamp
    return LightState(total, max(0.0, target - total), lamp_status(total, target))


def schedule_sessions_for_day(
    intervals: Sequence[tuple[time, time]], day: date, tz: ZoneInfo
) -> list[tuple[datetime, datetime]]:
    """Интервалы расписания (местное время) → сессии лампы на конкретный день."""
    return [
        (datetime.combine(day, s, tzinfo=tz), datetime.combine(day, e, tzinfo=tz))
        for s, e in sorted(intervals)
    ]


EVENING_DEFAULT = time(18)


def suggest_lamp_window(
    deficit_hours: float,
    sunset: datetime | None,
    lamp_ends: Sequence[datetime],
    day: date,
    tz: ZoneInfo,
) -> tuple[datetime, datetime, bool] | None:
    """Когда добрать недостающий свет: после заката и после последней работы лампы.
    Возвращает (начало, конец, упёрлись_в_полночь) или None, если света хватает."""
    if deficit_hours <= 0:
        return None
    starts = [e for e in lamp_ends if local_date(e, tz) == day]
    starts.append(sunset if sunset is not None else datetime.combine(day, EVENING_DEFAULT, tzinfo=tz))
    start = max(starts)
    midnight = local_midnight(day + timedelta(days=1), tz)
    end = start + timedelta(hours=deficit_hours)
    if end > midnight:
        return start, midnight, True
    return start, end, False


# ---------- пересадка ----------
def add_months(d: date, months: int) -> date:
    m = d.month - 1 + months
    y, m = d.year + m // 12, m % 12 + 1
    return date(y, m, min(d.day, calendar.monthrange(y, m)[1]))


@dataclass(frozen=True)
class RepotState:
    next_check_date: date
    due_in_days: int
    status: Status


def repot_state(
    last_repotted: datetime | None, added_at: datetime, months: int, now: datetime, tz: ZoneInfo
) -> RepotState:
    base = local_date(last_repotted or added_at, tz)
    nxt = add_months(base, months)
    due_in = (nxt - local_date(now, tz)).days
    return RepotState(nxt, due_in, status_for(due_in, REPOT_SOON_DAYS))


# ---------- статистика по неделям ----------
@dataclass(frozen=True)
class WeekStats:
    week_start: date
    waterings: int
    feedings: int
    lamp_hours: float
    is_current: bool
    sunshine_hours: float = 0.0


def weekly_stats(
    waterings: Sequence[datetime],
    feedings: Sequence[datetime],
    lamp_sessions: Sequence[Session],
    weeks: int,
    now: datetime,
    tz: ZoneInfo,
    sunshine: dict[date, float] | None = None,
) -> list[WeekStats]:
    """Последние `weeks` недель (с понедельника), последняя — текущая, неполная."""
    today = local_date(now, tz)
    current = today - timedelta(days=today.weekday())
    starts = [current - timedelta(weeks=i) for i in range(weeks - 1, -1, -1)]

    def week_of(dt: datetime) -> date:
        d = local_date(dt, tz)
        return d - timedelta(days=d.weekday())

    w_counts: dict[date, int] = {}
    for dt in waterings:
        w_counts[week_of(dt)] = w_counts.get(week_of(dt), 0) + 1
    f_counts: dict[date, int] = {}
    for dt in feedings:
        f_counts[week_of(dt)] = f_counts.get(week_of(dt), 0) + 1

    return [
        WeekStats(
            week_start=ws,
            waterings=w_counts.get(ws, 0),
            feedings=f_counts.get(ws, 0),
            lamp_hours=lamp_hours_between(
                lamp_sessions,
                local_midnight(ws, tz),
                local_midnight(ws + timedelta(days=7), tz),
                now,
            ),
            is_current=ws == current,
            sunshine_hours=sum(
                h for d, h in (sunshine or {}).items() if ws <= d < ws + timedelta(days=7)
            ),
        )
        for ws in starts
    ]
