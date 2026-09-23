from dataclasses import dataclass
from datetime import date, datetime, timedelta
from datetime import time as dtime
from zoneinfo import ZoneInfo

import pytest

from app.services.summary import (
    add_months,
    feed_state,
    lamp_hours_between,
    lamp_hours_in_day,
    lamp_hours_today,
    lamp_status,
    light_state,
    pick_next_fertilizer,
    repot_state,
    schedule_sessions_for_day,
    status_for,
    suggest_lamp_window,
    water_state,
    weekly_stats,
)

MSK = ZoneInfo("Europe/Moscow")


def msk(*args: int) -> datetime:
    return datetime(*args, tzinfo=MSK)


@dataclass
class FT:
    id: int
    name: str
    interval_days_active_season: int
    interval_days_dormant_season: int | None


LOMO = FT(1, "Lomonosoff", 14, 30)
BONA = FT(2, "Bona Forte", 10, None)
THIRD = FT(3, "Третий", 7, 21)


# ---------- статус ----------
@pytest.mark.parametrize(
    "due_in, ahead, expected",
    [(5, 1, "ok"), (1, 1, "soon"), (2, 2, "soon"), (0, 1, "late"), (-3, 1, "late"), (1, 0, "ok")],
)
def test_status_for(due_in, ahead, expected):
    assert status_for(due_in, ahead) == expected


# ---------- полив ----------
def test_water_counts_calendar_days_in_local_tz():
    # 23:30 по Москве вчера и 00:10 сегодня — это уже 1 день
    s = water_state(msk(2026, 9, 22, 23, 30), 4, 1, msk(2026, 9, 23, 0, 10), MSK)
    assert (s.days_since, s.due_in_days, s.status) == (1, 3, "ok")


def test_water_soon_and_late():
    now = msk(2026, 9, 23, 12)
    assert water_state(msk(2026, 9, 20, 9), 4, 1, now, MSK).status == "soon"
    assert water_state(msk(2026, 9, 19, 9), 4, 1, now, MSK).status == "late"


def test_water_never_watered_is_late():
    s = water_state(None, 4, 1, msk(2026, 9, 23, 12), MSK)
    assert (s.days_since, s.status) == (None, "late")


# ---------- чередование удобрений ----------
def test_next_fertilizer_first_when_no_history():
    assert pick_next_fertilizer([BONA, LOMO], None) is LOMO


def test_next_fertilizer_alternates():
    assert pick_next_fertilizer([LOMO, BONA], 1) is BONA
    assert pick_next_fertilizer([LOMO, BONA], 2) is LOMO


def test_next_fertilizer_cycles_through_three():
    types = [LOMO, BONA, THIRD]
    assert pick_next_fertilizer(types, 2) is THIRD
    assert pick_next_fertilizer(types, 3) is LOMO


def test_next_fertilizer_single_type_repeats():
    assert pick_next_fertilizer([LOMO], 1) is LOMO


def test_next_fertilizer_empty():
    assert pick_next_fertilizer([], None) is None


def test_next_fertilizer_after_deleted_type_goes_to_following_id():
    # последний тип (id=2) удалён — берём следующий по id
    assert pick_next_fertilizer([LOMO, THIRD], 2) is THIRD


# ---------- подкормка ----------
def test_feed_due_uses_next_type_interval_for_season():
    now = msk(2026, 9, 23, 12)
    s = feed_state(True, [LOMO, BONA], msk(2026, 9, 15, 9), 1, "active", 1, now, MSK)
    assert s.next is BONA
    assert s.interval_days == 10
    assert s.due_date == date(2026, 9, 25)
    assert (s.due_in_days, s.status, s.days_since) == (2, "ok", 8)


def test_feed_dormant_without_interval_is_off():
    s = feed_state(True, [LOMO, BONA], msk(2026, 9, 15), 1, "dormant", 1, msk(2026, 9, 23), MSK)
    assert s.next is BONA
    assert (s.status, s.due_date) == ("off", None)


def test_feed_disabled_is_off_but_still_suggests_type():
    s = feed_state(False, [LOMO, BONA], None, None, "active", 1, msk(2026, 9, 23), MSK)
    assert (s.status, s.next) == ("off", LOMO)


def test_feed_never_fed_is_due_today():
    s = feed_state(True, [LOMO], None, None, "active", 1, msk(2026, 9, 23), MSK)
    assert (s.due_in_days, s.status) == (0, "late")


# ---------- лампа ----------
def test_lamp_today_clips_to_local_midnight_and_counts_open_session():
    now = msk(2026, 9, 23, 10)
    sessions = [
        (msk(2026, 9, 22, 20), msk(2026, 9, 23, 2)),  # 2 ч после полуночи
        (msk(2026, 9, 23, 7), None),  # горит с 7:00 — 3 ч
    ]
    assert lamp_hours_today(sessions, now, MSK) == pytest.approx(5)


def test_lamp_overlapping_sessions_are_not_double_counted():
    # своя лампа 8–12 и общая 10–14 → 6 часов, а не 8
    sessions = [(msk(2026, 9, 23, 8), msk(2026, 9, 23, 12)), (msk(2026, 9, 23, 10), msk(2026, 9, 23, 14))]
    assert lamp_hours_today(sessions, msk(2026, 9, 23, 20), MSK) == pytest.approx(6)


def test_lamp_hours_between_window():
    sessions = [(msk(2026, 9, 21, 22), msk(2026, 9, 22, 3))]
    assert lamp_hours_between(sessions, msk(2026, 9, 22), msk(2026, 9, 23), msk(2026, 9, 30)) == pytest.approx(3)


@pytest.mark.parametrize(
    "hours, planned, expected",
    [(12, 12, "ok"), (11, 12, "ok"), (7, 12, "soon"), (5, 12, "late"), (0, 0, "ok")],
)
def test_lamp_status(hours, planned, expected):
    assert lamp_status(hours, planned) == expected


# ---------- пересадка ----------
def test_add_months_clamps_day():
    assert add_months(date(2026, 1, 31), 1) == date(2026, 2, 28)
    assert add_months(date(2026, 11, 15), 3) == date(2027, 2, 15)


def test_repot_from_last_repot():
    s = repot_state(msk(2026, 9, 2, 18), msk(2025, 1, 1), 6, msk(2026, 9, 23), MSK)
    assert s.next_check_date == date(2027, 3, 2)
    assert s.status == "ok"


def test_repot_falls_back_to_added_at_and_soon_within_two_weeks():
    s = repot_state(None, msk(2025, 10, 1), 12, msk(2026, 9, 23), MSK)
    assert (s.next_check_date, s.due_in_days, s.status) == (date(2026, 10, 1), 8, "soon")


# ---------- недельная статистика ----------
def test_weekly_stats_monday_weeks():
    now = msk(2026, 9, 23, 12)  # среда
    weeks = weekly_stats(
        waterings=[msk(2026, 9, 21, 9), msk(2026, 9, 20, 9), msk(2026, 9, 14, 9)],
        feedings=[msk(2026, 9, 22, 9)],
        lamp_sessions=[(msk(2026, 9, 20, 20), msk(2026, 9, 21, 2))],
        weeks=2,
        now=now,
        tz=MSK,
    )
    assert [w.week_start for w in weeks] == [date(2026, 9, 14), date(2026, 9, 21)]
    assert [w.waterings for w in weeks] == [2, 1]
    assert [w.feedings for w in weeks] == [0, 1]
    assert [w.lamp_hours for w in weeks] == pytest.approx([4, 2])
    assert weeks[-1].is_current and not weeks[0].is_current


def test_lamp_open_session_does_not_count_future():
    now = msk(2026, 9, 23, 10)
    assert lamp_hours_today([(now - timedelta(hours=1), None)], now, MSK) == pytest.approx(1)


# ---------- свет: расписание, план дня, подсказка ----------

def test_schedule_sessions_for_day_local_time():
    s = schedule_sessions_for_day([(dtime(7), dtime(10)), (dtime(17), dtime(21, 30))], date(2026, 9, 24), MSK)
    assert s == [(msk(2026, 9, 24, 7), msk(2026, 9, 24, 10)), (msk(2026, 9, 24, 17), msk(2026, 9, 24, 21, 30))]


def test_lamp_hours_in_day_plan_counts_future_scheduled_but_open_until_now():
    now = msk(2026, 9, 24, 12)
    sessions = [
        (msk(2026, 9, 24, 17), msk(2026, 9, 24, 21)),  # запланировано на вечер — 4 ч
        (msk(2026, 9, 24, 10), None),                   # вручную с 10:00, горит — 2 ч до «сейчас»
    ]
    assert lamp_hours_in_day(sessions, date(2026, 9, 24), now, MSK, plan=True) == pytest.approx(6)
    assert lamp_hours_in_day(sessions, date(2026, 9, 24), now, MSK, plan=False) == pytest.approx(2)


@pytest.mark.parametrize(
    "natural, lamp, target, total, deficit, status",
    [
        (1.5, 7, 13, 8.5, 4.5, "soon"),
        (10, 4, 13, 14, 0, "ok"),
        (0, 3, 13, 3, 10, "late"),
        (None, 12, 12, 12, 0, "ok"),  # город не задан — только лампа
    ],
)
def test_light_state(natural, lamp, target, total, deficit, status):
    s = light_state(target, natural, lamp)
    assert s.total_hours == pytest.approx(total)
    assert s.deficit_hours == pytest.approx(deficit)
    assert s.status == status


def test_suggest_window_starts_after_sunset_and_last_lamp():
    day = date(2026, 9, 24)
    w = suggest_lamp_window(2.5, msk(2026, 9, 24, 18, 23), [msk(2026, 9, 24, 19)], day, MSK)
    assert w == (msk(2026, 9, 24, 19), msk(2026, 9, 24, 21, 30), False)


def test_suggest_window_without_location_starts_evening():
    w = suggest_lamp_window(3, None, [], date(2026, 9, 24), MSK)
    assert w == (msk(2026, 9, 24, 18), msk(2026, 9, 24, 21), False)


def test_suggest_window_clamped_at_midnight():
    w = suggest_lamp_window(8, msk(2026, 9, 24, 18), [], date(2026, 9, 24), MSK)
    assert w == (msk(2026, 9, 24, 18), msk(2026, 9, 25, 0), True)


def test_suggest_window_none_when_enough():
    assert suggest_lamp_window(0, msk(2026, 9, 24, 18), [], date(2026, 9, 24), MSK) is None


def test_weekly_stats_sunshine():
    weeks = weekly_stats([], [], [], 1, msk(2026, 9, 23, 12), MSK, sunshine={date(2026, 9, 21): 5.5, date(2026, 9, 22): 1.0})
    assert weeks[0].sunshine_hours == pytest.approx(6.5)
