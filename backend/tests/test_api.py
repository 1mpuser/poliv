"""Интеграционные тесты API на реальном Postgres: изоляция данных учёток и админка.

Нужна отдельная база с «test» в имени — она пересоздаётся с нуля:
    TEST_DATABASE_URL=postgresql+psycopg://…/poliv_test pytest tests/test_api.py
Без переменной тесты пропускаются. Команда запуска в Docker — в CLAUDE.md.
"""

import os

import pytest

TEST_DB = os.environ.get("TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(not TEST_DB, reason="TEST_DATABASE_URL не задан")

if TEST_DB:
    if "test" not in TEST_DB.rsplit("/", 1)[-1]:
        raise RuntimeError("TEST_DATABASE_URL должен указывать на базу с «test» в имени — она будет стёрта")
    os.environ["DATABASE_URL"] = TEST_DB
    os.environ.setdefault("JWT_SECRET", "test-secret")


@pytest.fixture(scope="module")
def client():
    from alembic import command
    from alembic.config import Config
    from fastapi.testclient import TestClient
    from sqlalchemy import text

    from app.db import engine
    from app.main import app

    with engine.begin() as conn:
        conn.execute(text("DROP SCHEMA public CASCADE; CREATE SCHEMA public"))
    cfg = Config(os.path.join(os.path.dirname(__file__), "..", "alembic.ini"))
    cfg.set_main_option("script_location", os.path.join(os.path.dirname(__file__), "..", "alembic"))
    command.upgrade(cfg, "head")
    return TestClient(app)


def login(client, email, password):
    return client.post("/api/auth/token", data={"username": email, "password": password})


def auth(token):
    return {"Authorization": f"Bearer {token}"}


@pytest.fixture(scope="module")
def admin(client):
    """Владелец из миграции получает почту и пароль через set-owner."""
    from app.cli import set_owner

    set_owner("Owner@Example.com", "owner-pass-1")
    r = login(client, "owner@example.com", "owner-pass-1")
    assert r.status_code == 200
    return r.json()["access_token"]


def make_user(client, admin, email, password="user-pass-1"):
    r = client.post("/api/admin/users", json={"email": email, "password": password}, headers=auth(admin))
    assert r.status_code == 201, r.text
    return r.json()["id"], login(client, email, password).json()["access_token"]


def test_owner_keeps_seed_data(client, admin):
    names = [p["name"] for p in client.get("/api/plants", headers=auth(admin)).json()]
    assert names == ["Лимон", "Лайм"]
    me = client.get("/api/auth/me", headers=auth(admin)).json()
    assert me["email"] == "owner@example.com" and me["is_admin"]


def test_placeholder_owner_cannot_login(client):
    assert login(client, "owner@localhost.invalid", "!").status_code == 401


def test_new_user_starts_empty_with_defaults(client, admin):
    _, t = make_user(client, admin, "fresh@example.com")
    assert client.get("/api/plants", headers=auth(t)).json() == []
    ferts = [f["name"] for f in client.get("/api/fertilizers", headers=auth(t)).json()]
    assert ferts == ["Lomonosoff", "Bona Forte"]
    assert client.get("/api/settings", headers=auth(t)).json()["current_season"] == "active"


def test_data_isolation(client, admin):
    _, a = make_user(client, admin, "a@example.com")
    _, b = make_user(client, admin, "b@example.com")
    plant = client.post("/api/plants", json={"name": "Фикус A"}, headers=auth(a)).json()
    water = client.post("/api/waterings", json={"plant_id": plant["id"]}, headers=auth(a)).json()
    fert_a = client.get("/api/fertilizers", headers=auth(a)).json()[0]

    # B не видит и не трогает чужое — всё 404
    assert client.get("/api/plants", headers=auth(b)).json() == []
    for method, path, body in [
        ("GET", f"/api/plants/{plant['id']}", None),
        ("GET", f"/api/plants/{plant['id']}/summary", None),
        ("GET", f"/api/plants/{plant['id']}/history", None),
        ("PATCH", f"/api/plants/{plant['id']}", {"name": "взлом"}),
        ("DELETE", f"/api/plants/{plant['id']}", None),
        ("POST", "/api/waterings", {"plant_id": plant["id"]}),
        ("DELETE", f"/api/waterings/{water['id']}", None),
        ("POST", "/api/lamp-sessions/toggle", {"plant_id": plant["id"]}),
        ("PATCH", f"/api/fertilizers/{fert_a['id']}", {"name": "взлом"}),
    ]:
        r = client.request(method, path, json=body, headers=auth(b))
        assert r.status_code == 404, (method, path, r.status_code)
    assert client.get("/api/waterings", headers=auth(b)).json() == []

    # Подкормка своего растения чужим удобрением тоже 404
    plant_b = client.post("/api/plants", json={"name": "Фикус B"}, headers=auth(b)).json()
    r = client.post(
        "/api/feedings",
        json={"plant_id": plant_b["id"], "fertilizer_type_id": fert_a["id"], "method": "root"},
        headers=auth(b),
    )
    assert r.status_code == 404

    # Общая лампа — своя у каждой учётки
    assert client.post("/api/lamp-sessions/toggle", json={"plant_id": None}, headers=auth(a)).json()["is_on"]
    assert client.post("/api/lamp-sessions/toggle", json={"plant_id": None}, headers=auth(b)).json()["is_on"]
    summary_b = client.get(f"/api/plants/{plant_b['id']}/summary", headers=auth(b)).json()
    assert summary_b["lamp"]["shared_is_on"]
    assert client.get("/api/plants/summary", headers=auth(a)).json()[0]["plant"]["name"] == "Фикус A"


def test_admin_endpoints_hidden_from_users(client, admin):
    _, t = make_user(client, admin, "plain@example.com")
    assert client.get("/api/admin/users", headers=auth(t)).status_code == 404
    assert client.post("/api/admin/users", json={"email": "x@y.z", "password": "12345678"}, headers=auth(t)).status_code == 404


def test_admin_create_validation(client, admin):
    assert client.post("/api/admin/users", json={"email": "A@EXAMPLE.com", "password": "12345678"}, headers=auth(admin)).status_code == 409
    assert client.post("/api/admin/users", json={"email": "short@example.com", "password": "123"}, headers=auth(admin)).status_code == 400
    assert client.post("/api/admin/users", json={"email": "not-an-email", "password": "12345678"}, headers=auth(admin)).status_code == 400


def test_block_unblock_revokes_tokens(client, admin):
    uid, t = make_user(client, admin, "blocked@example.com")
    assert client.post(f"/api/admin/users/{uid}/block", headers=auth(admin)).status_code == 204
    assert client.get("/api/plants", headers=auth(t)).status_code == 401
    assert login(client, "blocked@example.com", "user-pass-1").status_code == 401
    client.post(f"/api/admin/users/{uid}/unblock", headers=auth(admin))
    assert login(client, "blocked@example.com", "user-pass-1").status_code == 200


def test_admin_password_reset_revokes_tokens(client, admin):
    uid, t = make_user(client, admin, "reset@example.com")
    r = client.post(f"/api/admin/users/{uid}/password", json={"password": "new-pass-22"}, headers=auth(admin))
    assert r.status_code == 204
    assert client.get("/api/plants", headers=auth(t)).status_code == 401
    assert login(client, "reset@example.com", "user-pass-1").status_code == 401
    assert login(client, "reset@example.com", "new-pass-22").status_code == 200


def test_admin_cannot_block_or_delete_self(client, admin):
    me = client.get("/api/auth/me", headers=auth(admin)).json()
    assert client.post(f"/api/admin/users/{me['id']}/block", headers=auth(admin)).status_code == 400
    assert client.delete(f"/api/admin/users/{me['id']}", headers=auth(admin)).status_code == 400


def test_delete_user_removes_data(client, admin):
    uid, t = make_user(client, admin, "gone@example.com")
    plant = client.post("/api/plants", json={"name": "Удалится"}, headers=auth(t)).json()
    assert client.delete(f"/api/admin/users/{uid}", headers=auth(admin)).status_code == 204
    assert login(client, "gone@example.com", "user-pass-1").status_code == 401
    from app.db import SessionLocal
    from app.models import Plant

    with SessionLocal() as db:
        assert db.get(Plant, plant["id"]) is None
    assert len(client.get("/api/plants", headers=auth(admin)).json()) == 2


def test_change_own_password(client, admin):
    _, t = make_user(client, admin, "self@example.com")
    bad = client.post("/api/auth/password", json={"current_password": "wrong", "new_password": "brand-new-1"}, headers=auth(t))
    assert bad.status_code == 400
    ok = client.post("/api/auth/password", json={"current_password": "user-pass-1", "new_password": "brand-new-1"}, headers=auth(t))
    assert ok.status_code == 200
    assert client.get("/api/plants", headers=auth(t)).status_code == 401
    assert client.get("/api/plants", headers=auth(ok.json()["access_token"])).status_code == 200


# ---------- свет и расписание лампы ----------
def _window_around_now():
    """Интервал ±1 ч вокруг текущего местного времени, не переходящий через полночь."""
    from datetime import datetime, time, timedelta

    from app.config import settings

    now = datetime.now(settings.zone)
    start = max(now - timedelta(hours=1), now.replace(hour=0, minute=0, second=0, microsecond=0))
    end = min(now + timedelta(hours=1), now.replace(hour=23, minute=59, second=0, microsecond=0))
    return start.strftime("%H:%M"), end.strftime("%H:%M")


def test_schedule_creates_sessions_and_toggle_ends_it(client, admin):
    _, t = make_user(client, admin, "lamp@example.com")
    plant = client.post("/api/plants", json={"name": "Лимон", "light_target_hours": 13}, headers=auth(t)).json()
    start, end = _window_around_now()
    r = client.put(
        "/api/lamp-schedules",
        json={"plant_id": plant["id"], "intervals": [{"start_time": start, "end_time": end}]},
        headers=auth(t),
    )
    assert r.status_code == 200 and len(r.json()) == 1

    s = client.get(f"/api/plants/{plant['id']}/summary", headers=auth(t)).json()
    assert s["lamp"]["is_on"]  # горит по расписанию
    assert s["light"]["natural_hours"] is None  # город не задан
    assert s["light"]["lamp_hours"] > 0 and s["light"]["target_hours"] == 13
    assert s["light"]["schedule"][0]["start_time"].startswith(start)

    # Кнопка гасит сессию по расписанию раньше срока, отмена возвращает конец
    off = client.post("/api/lamp-sessions/toggle", json={"plant_id": plant["id"]}, headers=auth(t)).json()
    assert off["is_on"] is False and off["previous_ended_at"] is not None
    assert not client.get(f"/api/plants/{plant['id']}/summary", headers=auth(t)).json()["lamp"]["is_on"]
    client.patch(f"/api/lamp-sessions/{off['session']['id']}", json={"ended_at": off["previous_ended_at"]}, headers=auth(t))
    assert client.get(f"/api/plants/{plant['id']}/summary", headers=auth(t)).json()["lamp"]["is_on"]

    # Повторная замена расписания не плодит дубли сессий на сегодня
    client.put("/api/lamp-schedules", json={"plant_id": plant["id"], "intervals": [{"start_time": start, "end_time": end}]}, headers=auth(t))
    sessions = client.get(f"/api/lamp-sessions?plant_id={plant['id']}", headers=auth(t)).json()
    assert len(sessions) == 1

    # Пустое расписание — сегодняшние сессии по нему уходят
    client.put("/api/lamp-schedules", json={"plant_id": plant["id"], "intervals": []}, headers=auth(t))
    assert client.get(f"/api/lamp-sessions?plant_id={plant['id']}", headers=auth(t)).json() == []


def test_schedule_validation_and_isolation(client, admin):
    _, a = make_user(client, admin, "sched-a@example.com")
    _, b = make_user(client, admin, "sched-b@example.com")
    plant = client.post("/api/plants", json={"name": "P"}, headers=auth(a)).json()
    bad = [
        [{"start_time": "10:00", "end_time": "09:00"}],
        [{"start_time": "07:00", "end_time": "10:00"}, {"start_time": "09:00", "end_time": "12:00"}],
    ]
    for intervals in bad:
        r = client.put("/api/lamp-schedules", json={"plant_id": plant["id"], "intervals": intervals}, headers=auth(a))
        assert r.status_code == 400
    r = client.put("/api/lamp-schedules", json={"plant_id": plant["id"], "intervals": []}, headers=auth(b))
    assert r.status_code == 404
    client.put("/api/lamp-schedules", json={"plant_id": None, "intervals": [{"start_time": "18:00", "end_time": "22:00"}]}, headers=auth(a))
    assert client.get("/api/lamp-schedules", headers=auth(b)).json() == []
    assert len(client.get("/api/lamp-schedules", headers=auth(a)).json()) == 1


def test_location_fetches_daylight(client, admin, monkeypatch):
    from datetime import date, datetime, timedelta

    from app.config import settings
    from app.services import light

    today = datetime.now(settings.zone).date()
    calls = []

    def fake_fetch(lat, lon):
        calls.append((lat, lon))
        return [
            {
                "day": today + timedelta(days=i),
                "sunrise": datetime.combine(today, datetime.min.time(), settings.zone) + timedelta(hours=6),
                "sunset": datetime.combine(today, datetime.min.time(), settings.zone) + timedelta(hours=18),
                "daylight_hours": 12.0,
                "sunshine_hours": 5.0,
            }
            for i in range(-2, 3)
        ]

    monkeypatch.setattr(light, "fetch_days", fake_fetch)
    _, t = make_user(client, admin, "sun@example.com")
    plant = client.post("/api/plants", json={"name": "P", "light_target_hours": 13}, headers=auth(t)).json()
    r = client.patch("/api/settings", json={"location_name": "Мытищи", "latitude": 55.91, "longitude": 37.73}, headers=auth(t))
    assert r.status_code == 200 and r.json()["location_name"] == "Мытищи"
    assert calls == [(55.91, 37.73)]
    assert client.get("/api/light/today", headers=auth(t)).json()["sunshine_hours"] == 5.0

    light_s = client.get(f"/api/plants/{plant['id']}/summary", headers=auth(t)).json()["light"]
    assert light_s["natural_hours"] == 5.0 and light_s["deficit_hours"] == 8.0
    weekly = client.get(f"/api/plants/{plant['id']}/stats/weekly?weeks=1", headers=auth(t)).json()
    assert weekly[0]["sunshine_hours"] > 0

    # Тот же город повторно — без лишнего запроса
    client.patch("/api/settings", json={"location_name": "Мытищи", "latitude": 55.91, "longitude": 37.73}, headers=auth(t))
    assert len(calls) == 1
