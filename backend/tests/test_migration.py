"""Миграция 0004: неявные лампы (своя растения / общая учётки) → объекты-лампы и периоды привязки.
Часы из истории не теряются, даже если растение стояло и под своей, и под общей лампой.
Как и test_api.py, пересоздаёт базу из TEST_DATABASE_URL."""

import os

import pytest

TEST_DB = os.environ.get("TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(not TEST_DB, reason="TEST_DATABASE_URL не задан")

if TEST_DB:
    if "test" not in TEST_DB.rsplit("/", 1)[-1]:
        raise RuntimeError("TEST_DATABASE_URL должен указывать на базу с «test» в имени — она будет стёрта")
    os.environ["DATABASE_URL"] = TEST_DB
    os.environ.setdefault("JWT_SECRET", "test-secret")


def _cfg():
    from alembic.config import Config

    here = os.path.dirname(__file__)
    cfg = Config(os.path.join(here, "..", "alembic.ini"))
    cfg.set_main_option("script_location", os.path.join(here, "..", "alembic"))
    return cfg


def test_0004_converts_implicit_lamps_without_losing_hours():
    from alembic import command
    from sqlalchemy import text

    from app.db import engine

    with engine.begin() as c:
        c.execute(text("DROP SCHEMA public CASCADE; CREATE SCHEMA public"))
    cfg = _cfg()
    command.upgrade(cfg, "0003")
    with engine.begin() as c:
        lemon = c.execute(text("SELECT id FROM plants WHERE name = 'Лимон'")).scalar_one()
        c.execute(
            text(
                "INSERT INTO lamp_schedules (user_id, plant_id, start_time, end_time) VALUES "
                "(1, :p, '07:00', '10:00'), (1, NULL, '18:00', '22:00')"
            ),
            {"p": lemon},
        )
        # сессии раньше added_at растения (сид 0001 создан «сейчас») — период должен их захватить
        c.execute(
            text(
                "INSERT INTO lamp_sessions (user_id, plant_id, started_at, ended_at, planned_hours_per_day) VALUES "
                "(1, :p, '2020-09-01 07:00+03', '2020-09-01 10:00+03', 12), "
                "(1, NULL, '2020-09-01 18:00+03', '2020-09-01 22:00+03', 12)"
            ),
            {"p": lemon},
        )

    command.upgrade(cfg, "head")
    with engine.begin() as c:
        lamps = dict(c.execute(text("SELECT name, mode::text FROM lamps WHERE user_id = 1")).all())
        assert lamps == {"Общая лампа": "schedule", "Лампа Лимон": "schedule"}
        periods = c.execute(
            text(
                "SELECT p.name, l.name, pl.ended_at IS NULL FROM plant_lamps pl "
                "JOIN plants p ON p.id = pl.plant_id JOIN lamps l ON l.id = pl.lamp_id ORDER BY 1, 2"
            )
        ).all()
        assert periods == [("Лайм", "Общая лампа", True), ("Лимон", "Лампа Лимон", True), ("Лимон", "Общая лампа", False)]
        sources = c.execute(text("SELECT source::text FROM lamp_sessions ORDER BY started_at")).scalars().all()
        assert sources == ["manual", "manual"]
        # часы лимона 1 сентября 2020: своя лампа 3 ч + общая 4 ч — обе в его периодах
        hours = c.execute(
            text(
                "SELECT sum(extract(epoch FROM (least(s.ended_at, coalesce(pl.ended_at, s.ended_at)) "
                "- greatest(s.started_at, pl.started_at))) / 3600) FROM lamp_sessions s "
                "JOIN plant_lamps pl ON pl.lamp_id = s.lamp_id WHERE pl.plant_id = :p"
            ),
            {"p": lemon},
        ).scalar_one()
        assert float(hours) == 7.0

    command.downgrade(cfg, "0003")
    with engine.begin() as c:
        owners = c.execute(text("SELECT plant_id FROM lamp_sessions ORDER BY started_at")).scalars().all()
        assert owners == [lemon, None]
    command.upgrade(cfg, "head")
