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

    # Лампы — свои у каждой учётки
    lamp_a = client.post("/api/lamps", json={"name": "Лампа A", "plant_ids": [plant["id"]]}, headers=auth(a)).json()
    for method, path, body in [
        ("GET", f"/api/lamps/{lamp_a['id']}", None),
        ("PATCH", f"/api/lamps/{lamp_a['id']}", {"name": "взлом"}),
        ("DELETE", f"/api/lamps/{lamp_a['id']}", None),
        ("POST", f"/api/lamps/{lamp_a['id']}/toggle", None),
        ("PUT", f"/api/lamps/{lamp_a['id']}/schedule", {"intervals": []}),
        ("PUT", f"/api/plants/{plant_b['id']}/lamp", {"lamp_id": lamp_a["id"]}),
        ("POST", "/api/lamps", {"name": "чужое растение", "plant_ids": [plant["id"]]}),
        ("POST", "/api/lamp-sessions", {"lamp_id": lamp_a["id"]}),
    ]:
        r = client.request(method, path, json=body, headers=auth(b))
        assert r.status_code == 404, (method, path, r.status_code)
    assert client.get("/api/lamps", headers=auth(b)).json() == []
    assert client.post(f"/api/lamps/{lamp_a['id']}/toggle", headers=auth(a)).json()["is_on"]
    assert client.get(f"/api/plants/{plant['id']}/summary", headers=auth(a)).json()["lamp"]["is_on"]


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


def test_schedule_lamp_lights_its_plants_and_toggle_ends_it(client, admin):
    _, t = make_user(client, admin, "lamp@example.com")
    lemon = client.post("/api/plants", json={"name": "Лимон", "light_target_hours": 13}, headers=auth(t)).json()
    lime = client.post("/api/plants", json={"name": "Лайм"}, headers=auth(t)).json()
    lamp = client.post(
        "/api/lamps",
        json={"name": "Лампа цитрусы", "mode": "schedule", "plant_ids": [lemon["id"], lime["id"]]},
        headers=auth(t),
    ).json()
    assert lamp["plant_ids"] == sorted([lemon["id"], lime["id"]])
    start, end = _window_around_now()
    r = client.put(f"/api/lamps/{lamp['id']}/schedule", json={"intervals": [{"start_time": start, "end_time": end}]}, headers=auth(t))
    assert r.status_code == 200 and len(r.json()) == 1

    for p in (lemon, lime):
        s = client.get(f"/api/plants/{p['id']}/summary", headers=auth(t)).json()
        assert s["lamp"]["is_on"] and s["light"]["lamp_hours"] > 0
        assert s["light"]["lamp"]["name"] == "Лампа цитрусы"

    # Кнопка у лимона гасит общую с лаймом лампу, отмена возвращает конец
    off = client.post("/api/lamp-sessions/toggle", json={"plant_id": lemon["id"]}, headers=auth(t)).json()
    assert off["is_on"] is False and off["previous_ended_at"] is not None
    assert not client.get(f"/api/plants/{lime['id']}/summary", headers=auth(t)).json()["lamp"]["is_on"]
    client.patch(f"/api/lamp-sessions/{off['session']['id']}", json={"ended_at": off["previous_ended_at"]}, headers=auth(t))
    assert client.get(f"/api/plants/{lime['id']}/summary", headers=auth(t)).json()["lamp"]["is_on"]

    # Повторная замена расписания не плодит дубли, пустое — убирает сегодняшние сессии
    client.put(f"/api/lamps/{lamp['id']}/schedule", json={"intervals": [{"start_time": start, "end_time": end}]}, headers=auth(t))
    assert len(client.get(f"/api/lamp-sessions?lamp_id={lamp['id']}", headers=auth(t)).json()) == 1
    client.put(f"/api/lamps/{lamp['id']}/schedule", json={"intervals": []}, headers=auth(t))
    assert client.get(f"/api/lamp-sessions?lamp_id={lamp['id']}", headers=auth(t)).json() == []

    # Расписание — только в режиме «По расписанию»
    client.patch(f"/api/lamps/{lamp['id']}", json={"mode": "manual"}, headers=auth(t))
    r = client.put(f"/api/lamps/{lamp['id']}/schedule", json={"intervals": [{"start_time": "18:00", "end_time": "22:00"}]}, headers=auth(t))
    assert r.status_code == 400


def test_lamp_validation(client, admin):
    _, t = make_user(client, admin, "sched@example.com")
    lamp = client.post("/api/lamps", json={"name": "L", "mode": "schedule"}, headers=auth(t)).json()
    bad = [
        [{"start_time": "10:00", "end_time": "09:00"}],
        [{"start_time": "07:00", "end_time": "10:00"}, {"start_time": "09:00", "end_time": "12:00"}],
    ]
    for intervals in bad:
        r = client.put(f"/api/lamps/{lamp['id']}/schedule", json={"intervals": intervals}, headers=auth(t))
        assert r.status_code == 400
    r = client.patch(f"/api/lamps/{lamp['id']}", json={"morning_not_before": "23:00", "evening_not_after": "06:00"}, headers=auth(t))
    assert r.status_code == 400
    # «Авто» без города — нельзя
    r = client.post("/api/lamps", json={"name": "A", "mode": "auto"}, headers=auth(t))
    assert r.status_code == 400 and "город" in r.json()["detail"]


def test_plant_without_lamp_cannot_toggle(client, admin):
    _, t = make_user(client, admin, "nolamp@example.com")
    plant = client.post("/api/plants", json={"name": "P"}, headers=auth(t)).json()
    r = client.post("/api/lamp-sessions/toggle", json={"plant_id": plant["id"]}, headers=auth(t))
    assert r.status_code == 400 and "лампы" in r.json()["detail"]
    s = client.get(f"/api/plants/{plant['id']}/summary", headers=auth(t)).json()
    assert s["light"]["lamp"] is None and s["lamp"]["is_on"] is False


def test_move_and_archive_keep_history(client, admin):
    _, t = make_user(client, admin, "move@example.com")
    plant = client.post("/api/plants", json={"name": "P"}, headers=auth(t)).json()
    a = client.post("/api/lamps", json={"name": "A", "plant_ids": [plant["id"]]}, headers=auth(t)).json()
    b = client.post("/api/lamps", json={"name": "B"}, headers=auth(t)).json()

    def flick(lamp_id):
        client.post(f"/api/lamps/{lamp_id}/toggle", headers=auth(t))
        client.post(f"/api/lamps/{lamp_id}/toggle", headers=auth(t))

    def lamp_names():
        events = client.get(f"/api/plants/{plant['id']}/history?types=lamp", headers=auth(t)).json()
        return sorted(e["lamp_name"] for e in events)

    flick(a["id"])
    assert lamp_names() == ["A"]
    r = client.put(f"/api/plants/{plant['id']}/lamp", json={"lamp_id": b["id"]}, headers=auth(t))
    assert r.status_code == 200
    flick(a["id"])  # A светит уже без растения — в его историю не попадает
    flick(b["id"])
    assert lamp_names() == ["A", "B"]

    assert client.delete(f"/api/lamps/{a['id']}", headers=auth(t)).status_code == 204
    assert client.get(f"/api/lamps/{a['id']}", headers=auth(t)).status_code == 404
    assert [l["name"] for l in client.get("/api/lamps", headers=auth(t)).json()] == ["B"]
    assert lamp_names() == ["A", "B"]


def test_yandex_token_is_encrypted_and_never_returned(client, admin, monkeypatch):
    from app.db import SessionLocal
    from app.models import UserSettings
    from app.services import yandex

    def fake_list(token):
        if token != "y0_good-token-123":
            raise yandex.YandexAuthError("Яндекс не принял токен")
        return [yandex.Device("dev-1", "Лампа цитрусы", "Спальня", "devices.types.socket")]

    monkeypatch.setattr(yandex, "list_devices", fake_list)
    uid, t = make_user(client, admin, "token@example.com")
    assert client.get("/api/settings", headers=auth(t)).json()["yandex_status"] == "none"
    assert client.get("/api/yandex/devices", headers=auth(t)).status_code == 400
    assert client.put("/api/settings/yandex-token", json={"token": "y0_bad-token-123"}, headers=auth(t)).status_code == 400

    r = client.put("/api/settings/yandex-token", json={"token": "y0_good-token-123"}, headers=auth(t))
    assert r.status_code == 200 and r.json()["yandex_status"] == "ok" and "y0_good" not in r.text
    assert client.get("/api/yandex/devices", headers=auth(t)).json() == [
        {"id": "dev-1", "name": "Лампа цитрусы", "room": "Спальня", "type": "devices.types.socket"}
    ]
    with SessionLocal() as db:
        stored = db.get(UserSettings, uid).yandex_token
        assert stored and "y0_good" not in stored
    r = client.put("/api/settings/yandex-token", json={"token": None}, headers=auth(t))
    assert r.json()["yandex_status"] == "none"


def test_auto_lamp_plans_and_drives_plug(client, admin, monkeypatch):
    from datetime import date, datetime

    from sqlalchemy import select

    from app.config import settings
    from app.db import SessionLocal
    from app.models import DaylightDay, Lamp, LampMode, LampSession, LampSource
    from app.services import lamps, light, yandex

    tz = settings.zone
    day = date(2030, 1, 15)

    def at(h, m=0):
        return datetime(2030, 1, 15, h, m, tzinfo=tz)

    calls = []
    monkeypatch.setattr(light, "fetch_days", lambda lat, lon: [])
    monkeypatch.setattr(yandex, "list_devices", lambda token: [])
    monkeypatch.setattr(yandex, "set_on", lambda token, dev, on: calls.append((dev, on)))

    uid, t = make_user(client, admin, "auto@example.com")
    lemon = client.post("/api/plants", json={"name": "Лимон", "light_target_hours": 12}, headers=auth(t)).json()
    lime = client.post("/api/plants", json={"name": "Лайм", "light_target_hours": 10}, headers=auth(t)).json()
    client.patch("/api/settings", json={"location_name": "Москва", "latitude": 55.75, "longitude": 37.62}, headers=auth(t))
    client.put("/api/settings/yandex-token", json={"token": "y0_test-token-123"}, headers=auth(t))
    lamp = client.post(
        "/api/lamps",
        json={"name": "Лампа цитрусы", "mode": "auto", "device_id": "dev-1", "device_name": "Розетка",
              "plant_ids": [lemon["id"], lime["id"]]},
        headers=auth(t),
    )
    assert lamp.status_code == 201, lamp.text
    lamp_id = lamp.json()["id"]

    def planned(db):
        q = select(LampSession).where(LampSession.lamp_id == lamp_id, LampSession.source == LampSource.auto).order_by(LampSession.started_at)
        return [(s.started_at, s.ended_at) for s in db.scalars(q)]

    with SessionLocal() as db:
        db.merge(DaylightDay(user_id=uid, day=day, sunrise=at(9), sunset=at(16, 30), daylight_hours=7.5, sunshine_hours=2.0))
        db.commit()
        calls.clear()  # при создании лампы розетке уже ушло «выкл»

        lamps.tick(db, now=at(5))
        # лимону не хватает 10 ч: утро — половина, но не раньше 06:00 (3 ч); вечер — остаток 7 ч, но до 23:00;
        # день — оставшиеся после них 0,5 ч вплотную перед закатом
        assert planned(db) == [(at(6), at(9)), (at(16), at(16, 30)), (at(16, 30), at(23))]
        assert calls == []  # розетка уже выключена

        lamps.tick(db, now=at(6, 30))
        assert calls == [("dev-1", True)]
        lamps.tick(db, now=at(6, 31))
        assert calls == [("dev-1", True)]  # команда только при смене состояния

        # Выключили кнопкой посреди утра — утро не создаётся заново
        lamps.toggle(db, db.get(Lamp, lamp_id), at(6, 40))
        assert calls[-1] == ("dev-1", False)
        lamps.tick(db, now=at(6, 41))
        assert calls[-1] == ("dev-1", False) and len(planned(db)) == 3
        # дневная часть пересчитана с учётом того, что утро недосветило: начинается раньше 16:00
        assert planned(db)[1][1] == at(16, 30) and planned(db)[1][0] < at(16)

        # Смена режима во время вечерней досветки: идущая гаснет, розетке «выкл»
        lamps.tick(db, now=at(17))
        assert calls[-1] == ("dev-1", True)
        lamps.update(db, db.get(Lamp, lamp_id), {"mode": LampMode.manual}, None, at(17, 5))
        assert planned(db)[-1] == (at(16, 30), at(17, 5))
        assert calls[-1] == ("dev-1", False)


def test_auto_lamp_plans_daytime_part(client, admin, monkeypatch):
    from datetime import date, datetime

    from sqlalchemy import select

    from app.config import settings
    from app.db import SessionLocal
    from app.models import DaylightDay, LampSession, LampSource
    from app.services import lamps, light, yandex

    tz = settings.zone
    day = date(2030, 1, 20)

    def at(h, m=0):
        return datetime(2030, 1, 20, h, m, tzinfo=tz)

    calls = []
    monkeypatch.setattr(light, "fetch_days", lambda lat, lon: [])
    monkeypatch.setattr(yandex, "list_devices", lambda token: [])
    monkeypatch.setattr(yandex, "set_on", lambda token, dev, on: calls.append((dev, on)))

    uid, t = make_user(client, admin, "day@example.com")
    lemon = client.post("/api/plants", json={"name": "Лимон", "light_target_hours": 12}, headers=auth(t)).json()
    client.patch("/api/settings", json={"location_name": "Москва", "latitude": 55.75, "longitude": 37.62}, headers=auth(t))
    client.put("/api/settings/yandex-token", json={"token": "y0_test-token-123"}, headers=auth(t))
    # лампу создали днём в пасмурный день
    lamp = client.post(
        "/api/lamps",
        json={"name": "Дневная", "mode": "auto", "device_id": "dev-2", "device_name": "Розетка",
              "plant_ids": [lemon["id"]]},
        headers=auth(t),
    ).json()
    lamp_id = lamp["id"]

    def planned(db):
        q = select(LampSession).where(LampSession.lamp_id == lamp_id, LampSession.source == LampSource.auto).order_by(LampSession.started_at)
        return [(s.started_at, s.ended_at) for s in db.scalars(q)]

    with SessionLocal() as db:
        db.merge(DaylightDay(user_id=uid, day=day, sunrise=at(9), sunset=at(16, 30), daylight_hours=7.5, sunshine_hours=2.0))
        db.commit()
        calls.clear()  # при создании лампы розетке уже ушло «выкл»

        # на первом шаге днём появляется дневная часть с текущей минуты, розетке — «вкл»
        lamps.tick(db, now=at(14))
        assert planned(db) == [(at(14), at(16, 30)), (at(16, 30), at(23))]
        assert calls == [("dev-2", True)]


def test_plug_errors_and_archive(client, admin, monkeypatch):
    from app.db import SessionLocal
    from app.services import lamps, yandex

    state = {"fail": True}
    calls = []

    def fake_set_on(token, dev, on):
        calls.append((dev, on))
        if state["fail"]:
            raise yandex.YandexError("Умный дом Яндекса недоступен")

    monkeypatch.setattr(yandex, "set_on", fake_set_on)
    monkeypatch.setattr(yandex, "list_devices", lambda token: [])
    _, t = make_user(client, admin, "plug@example.com")
    plant = client.post("/api/plants", json={"name": "P"}, headers=auth(t)).json()

    # Розетка задана, а токена нет — ошибка в лампе, без падения
    lamp = client.post("/api/lamps", json={"name": "L", "device_id": "dev-9", "plant_ids": [plant["id"]]}, headers=auth(t)).json()
    assert lamp["last_error"] == "Не задан токен Яндекса"
    client.put("/api/settings/yandex-token", json={"token": "y0_test-token-123"}, headers=auth(t))

    on = client.post(f"/api/lamps/{lamp['id']}/toggle", headers=auth(t)).json()
    assert on["is_on"] and on["plug_error"] == "Умный дом Яндекса недоступен"  # сессия записана
    got = client.get(f"/api/lamps/{lamp['id']}", headers=auth(t)).json()
    assert got["last_state"] is None and got["last_error"] == "Умный дом Яндекса недоступен"

    state["fail"] = False
    with SessionLocal() as db:
        lamps.tick(db)  # повтор на следующем шаге
    got = client.get(f"/api/lamps/{lamp['id']}", headers=auth(t)).json()
    assert got["last_state"] is True and got["last_error"] is None

    # Удалили горящую лампу — розетке «выкл»
    assert client.delete(f"/api/lamps/{lamp['id']}", headers=auth(t)).status_code == 204
    assert calls[-1] == ("dev-9", False)

    # 401 от Яндекса — токен помечается недействительным
    lamp2 = client.post("/api/lamps", json={"name": "L2", "device_id": "dev-8"}, headers=auth(t)).json()

    def auth_fail(token, dev, on):
        raise yandex.YandexAuthError("Яндекс не принял токен")

    monkeypatch.setattr(yandex, "set_on", auth_fail)
    client.post(f"/api/lamps/{lamp2['id']}/toggle", headers=auth(t))
    assert client.get("/api/settings", headers=auth(t)).json()["yandex_status"] == "invalid"


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
