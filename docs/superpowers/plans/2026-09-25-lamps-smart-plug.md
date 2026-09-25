# Лампы как объект и умная розетка через Яндекс — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Лампа становится объектом учётки с привязанными растениями (по периодам), а лампа с розеткой в Умном доме Яндекса сама досвечивает растения до нормы: половина нехватки утром до рассвета, остаток вечером после заката.

**Architecture:** Новые таблицы `lamps` и `plant_lamps` (периоды привязки); `lamp_sessions`/`lamp_schedules` ссылаются на лампу. Чистые правила досветки — в `services/summary.py`; БД-логика ламп (привязка, досветка, розетка, шаг раз в минуту) — новый `services/lamps.py`; HTTP к Яндексу — `services/yandex.py`; шифрование токена — `services/secret_box.py`. Розетке команда уходит только при смене нужного состояния (`lamps.last_state`).

**Tech Stack:** FastAPI, SQLAlchemy 2, Alembic, Postgres, `cryptography` (Fernet + HKDF), urllib; React + TypeScript (Vite), без UI-библиотек.

**Spec:** `docs/superpowers/specs/2026-09-25-lamps-smart-plug-design.md`

## Global Constraints

- Ответы, тексты интерфейса, комментарии, сообщения ошибок API — на русском.
- В коммитах — без трейлеров `Co-Authored-By` и любой AI-атрибуции.
- Схему БД менять только новой миграцией (`0004_lamps.py`), старые миграции не трогать.
- Любой доступ по id — через `crud.owned_*`: чужая или архивная лампа отвечает 404.
- Время — `timestamptz`, дни — календарные в `settings.zone` (Europe/Moscow).
- Фронт: только классы и токены `design/` + блок дополнений в конце `styles.css`; числа/даты — `format.ts`; запросы — только `api.ts`.
- Тесты в сеть не ходят: `light.fetch_days`, `yandex.set_on`, `yandex.list_devices` подменяются `monkeypatch`.
- Токен Яндекса хранится только зашифрованным, в ответах API не возвращается; ключ — HKDF от `JWT_SECRET`.
- Границы по умолчанию: утро не раньше 06:00, вечер не позже 23:00.

## Review Focus

1. Розетка задана, а токена нет — ошибка «Не задан токен Яндекса» пишется в лампу, ничего не падает (тест в Task 5).
2. Лампу, которая горит, удалили (архив) — розетке уходит «выкл» (тест в Task 5).
3. Досветку выключили кнопкой посреди утра — на следующем шаге утро не создаётся заново (тест в Task 5).
4. Режим сменили с «Авто» на «Вручную» во время досветки — идущая досветка гаснет, будущая удаляется (тест в Task 5).
5. Кнопка «Лампа» у растения без лампы — понятная 400, а не 500 (тест в Task 6).

## Карта файлов

Backend:
- Create `backend/alembic/versions/0004_lamps.py` — схема и перенос данных.
- Modify `backend/app/models.py` — `LampMode`, `LampSource`, `Lamp`, `PlantLamp`; `lamp_id`/`source` в сессиях и расписаниях; токен в `UserSettings`.
- Modify `backend/app/services/summary.py` — `clip_session`, `lamp_need`, `plan_morning`, `plan_evening`.
- Create `backend/app/services/secret_box.py` — шифрование токена.
- Create `backend/app/services/yandex.py` — API Умного дома.
- Create `backend/app/services/lamps.py` — привязка, сессии растения, переключение, досветка, розетка, шаг.
- Modify `backend/app/services/light.py` — расписания по `lamp_id`; `sync_all` только свет.
- Modify `backend/app/services/plants.py` — сводка/история/статистика через периоды.
- Modify `backend/app/schemas.py`, `backend/app/crud.py`.
- Create `backend/app/routers/lamps.py` — `/lamps`, `/yandex/devices`.
- Modify `backend/app/routers/lamp.py`, `routers/light.py`, `routers/plants.py`, `routers/settings.py`, `backend/app/main.py`.
- Modify `backend/requirements.txt` — `cryptography`.
- Tests: `backend/tests/test_summary.py`, create `backend/tests/test_secret_box.py`, `backend/tests/test_yandex.py`, `backend/tests/test_migration.py`; modify `backend/tests/test_api.py`.

Frontend:
- Modify `frontend/src/types.ts`, `api.ts`, `components/LampScheduleEditor.tsx`, `components/LightSettings.tsx`, `components/LightHint.tsx`, `components/History.tsx`, `components/PlantActions.tsx`, `pages/Dashboard.tsx`, `pages/SettingsPage.tsx`, `styles.css`.
- Create `frontend/src/components/LampList.tsx`.

Docs: `docs/architecture.md`, `api.md`, `usage.md`, `decisions.md`, `history.md`, `ideas.md`, `CLAUDE.md`.

Команды тестов (из `CLAUDE.md`):
- юнит: `cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q`
- все, с Postgres (стек должен быть запущен: `docker compose up -d --build`):
  ```bash
  docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test' 2>/dev/null
  docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
    'export TEST_DATABASE_URL="${DATABASE_URL%/*}/poliv_test"; pip install -q pytest httpx cryptography && python -m pytest -q'
  ```
  (`cryptography` в команде — пока образ backend не пересобран с новым `requirements.txt`; после `docker compose build backend` можно убрать.)

---

### Task 1: Чистые правила досветки

**Files:**
- Modify: `backend/app/services/summary.py` (после `suggest_lamp_window`)
- Test: `backend/tests/test_summary.py`

**Interfaces:**
- Produces:
  - `Period = tuple[datetime, datetime | None]`
  - `clip_session(start: datetime, end: datetime | None, periods: Sequence[Period]) -> list[Session]`
  - `lamp_need(deficits: Iterable[float]) -> float`
  - `plan_morning(need: float, sunrise: datetime | None, not_before: time, day: date, tz: ZoneInfo) -> tuple[datetime, datetime] | None`
  - `plan_evening(remaining: float, sunset: datetime | None, not_after: time, day: date, tz: ZoneInfo) -> tuple[datetime, datetime] | None`

- [ ] **Step 1: Write the failing tests** — дописать в импорт `test_summary.py` `clip_session, lamp_need, plan_evening, plan_morning` и в конец файла:

```python
# ---------- периоды привязки и досветка ----------
def test_clip_session_to_periods():
    periods = [(msk(2026, 1, 1, 8), msk(2026, 1, 1, 10)), (msk(2026, 1, 1, 12), None)]
    assert clip_session(msk(2026, 1, 1, 7), msk(2026, 1, 1, 13), periods) == [
        (msk(2026, 1, 1, 8), msk(2026, 1, 1, 10)),
        (msk(2026, 1, 1, 12), msk(2026, 1, 1, 13)),
    ]
    # горящая сессия: закрытый период обрезает её своим концом, в открытом она горит дальше
    assert clip_session(msk(2026, 1, 1, 9), None, periods) == [
        (msk(2026, 1, 1, 9), msk(2026, 1, 1, 10)),
        (msk(2026, 1, 1, 12), None),
    ]
    # между периодами растение под лампой не стояло
    assert clip_session(msk(2026, 1, 1, 10, 30), msk(2026, 1, 1, 11), periods) == []


def test_lamp_need_is_max_deficit():
    assert lamp_need([4.0, 2.0]) == 4.0
    assert lamp_need([]) == 0.0
    assert lamp_need([0.0]) == 0.0


D = date(2026, 1, 15)


def test_plan_morning_half_ends_at_sunrise():
    assert plan_morning(4, msk(2026, 1, 15, 9), dtime(6), D, MSK) == (msk(2026, 1, 15, 7), msk(2026, 1, 15, 9))


def test_plan_morning_not_before_bound():
    assert plan_morning(10, msk(2026, 1, 15, 9), dtime(6), D, MSK) == (msk(2026, 1, 15, 6), msk(2026, 1, 15, 9))


def test_plan_morning_none_in_summer_or_when_enough():
    summer = date(2026, 6, 15)
    assert plan_morning(4, msk(2026, 6, 15, 4, 30), dtime(6), summer, MSK) is None
    assert plan_morning(0, msk(2026, 1, 15, 9), dtime(6), D, MSK) is None
    assert plan_morning(4, None, dtime(6), D, MSK) is None


def test_plan_evening_from_sunset_bounded():
    sunset = msk(2026, 1, 15, 16, 30)
    assert plan_evening(2.5, sunset, dtime(23), D, MSK) == (sunset, msk(2026, 1, 15, 19))
    assert plan_evening(8, sunset, dtime(23), D, MSK) == (sunset, msk(2026, 1, 15, 23))
    assert plan_evening(0, sunset, dtime(23), D, MSK) is None
    assert plan_evening(2, msk(2026, 1, 15, 23, 30), dtime(23), D, MSK) is None
    assert plan_evening(2, None, dtime(23), D, MSK) is None
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q tests/test_summary.py`
Expected: FAIL — `ImportError: cannot import name 'clip_session'`.

- [ ] **Step 3: Implement** — в `summary.py`: в импорт `from collections.abc import Iterable, Sequence`; после `suggest_lamp_window`:

```python
# ---------- лампы: периоды привязки и досветка до нормы ----------
Period = tuple[datetime, datetime | None]  # растение стояло под лампой; конец None — стоит сейчас


def clip_session(start: datetime, end: datetime | None, periods: Sequence[Period]) -> list[Session]:
    """Части сессии лампы, пока растение стояло под ней. Закрытый период обрезает
    и горящую сессию; в открытом горящая остаётся открытой."""
    parts: list[Session] = []
    for p_start, p_end in periods:
        s = max(start, p_start)
        if end is None:
            e = p_end
        elif p_end is None:
            e = end
        else:
            e = min(end, p_end)
        if e is None or e > s:
            parts.append((s, e))
    return parts


def lamp_need(deficits: Iterable[float]) -> float:
    """Сколько досвечивать лампе: по самому требовательному растению под ней."""
    return max([0.0, *deficits])


def plan_morning(
    need: float, sunrise: datetime | None, not_before: time, day: date, tz: ZoneInfo
) -> tuple[datetime, datetime] | None:
    """Утренняя половина досветки: заканчивается на рассвете, начинается не раньше not_before."""
    if need <= 0 or sunrise is None:
        return None
    start = max(sunrise - timedelta(hours=need / 2), datetime.combine(day, not_before, tzinfo=tz))
    return (start, sunrise) if start < sunrise else None


def plan_evening(
    remaining: float, sunset: datetime | None, not_after: time, day: date, tz: ZoneInfo
) -> tuple[datetime, datetime] | None:
    """Вечерний остаток: с заката, но не позже not_after."""
    if remaining <= 0 or sunset is None:
        return None
    end = min(sunset + timedelta(hours=remaining), datetime.combine(day, not_after, tzinfo=tz))
    return (sunset, end) if end > sunset else None
```

- [ ] **Step 4: Run to verify pass** — та же команда. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/app/services/summary.py backend/tests/test_summary.py
git commit -m "Свет: чистые правила досветки — периоды привязки, утро/вечер"
```

---

### Task 2: Шифрование токена и клиент Умного дома Яндекса

**Files:**
- Modify: `backend/requirements.txt`
- Create: `backend/app/services/secret_box.py`, `backend/app/services/yandex.py`
- Test: create `backend/tests/test_secret_box.py`, `backend/tests/test_yandex.py`

**Interfaces:**
- Produces:
  - `secret_box.encrypt(plain: str, secret: str) -> str`; `secret_box.decrypt(token: str, secret: str) -> str | None` (чужой ключ/мусор → `None`)
  - `yandex.Device(id: str, name: str, room: str | None, type: str)` (frozen dataclass)
  - `yandex.YandexError(Exception)`, `yandex.YandexAuthError(YandexError)`
  - `yandex.list_devices(token: str) -> list[Device]` — только устройства с `devices.capabilities.on_off`
  - `yandex.set_on(token: str, device_id: str, on: bool) -> None`

- [ ] **Step 1: Add dependency** — в `backend/requirements.txt` строку `cryptography>=42`.

- [ ] **Step 2: Write the failing tests**

`backend/tests/test_secret_box.py`:

```python
from app.services.secret_box import decrypt, encrypt


def test_roundtrip_and_ciphertext_hides_token():
    box = encrypt("y0_secret-token", "jwt-secret")
    assert "y0_secret" not in box
    assert decrypt(box, "jwt-secret") == "y0_secret-token"


def test_other_secret_or_garbage_gives_none():
    box = encrypt("y0_secret-token", "jwt-secret")
    assert decrypt(box, "другой секрет") is None
    assert decrypt("мусор", "jwt-secret") is None
```

`backend/tests/test_yandex.py`:

```python
import io
import json
import urllib.error

import pytest

from app.services import yandex


class FakeUrlopen:
    def __init__(self, payload=None, error=None):
        self.payload, self.error, self.requests = payload, error, []

    def __call__(self, req, timeout):
        self.requests.append(req)
        if self.error is not None:
            raise self.error
        return io.BytesIO(json.dumps(self.payload).encode())


def http_error(code):
    return urllib.error.HTTPError("https://api.iot.yandex.net", code, "err", {}, None)


USER_INFO = {
    "status": "ok",
    "rooms": [{"id": "r1", "name": "Спальня"}],
    "devices": [
        {"id": "d1", "name": "Лампа цитрусы", "room": "r1", "type": "devices.types.socket",
         "capabilities": [{"type": "devices.capabilities.on_off"}]},
        {"id": "d2", "name": "Датчик", "room": None, "type": "devices.types.sensor", "capabilities": []},
    ],
}


def test_list_devices_keeps_switchable_with_room(monkeypatch):
    fake = FakeUrlopen(USER_INFO)
    monkeypatch.setattr(yandex.urllib.request, "urlopen", fake)
    assert yandex.list_devices("tok") == [yandex.Device("d1", "Лампа цитрусы", "Спальня", "devices.types.socket")]
    assert fake.requests[0].full_url == "https://api.iot.yandex.net/v1.0/user/info"
    assert fake.requests[0].get_header("Authorization") == "Bearer tok"


def test_set_on_sends_on_off_action(monkeypatch):
    fake = FakeUrlopen({"status": "ok", "devices": [{"id": "d1", "capabilities": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "action_result": {"status": "DONE"}}}]}]})
    monkeypatch.setattr(yandex.urllib.request, "urlopen", fake)
    yandex.set_on("tok", "d1", True)
    req = fake.requests[0]
    assert req.get_method() == "POST" and req.full_url.endswith("/v1.0/devices/actions")
    assert json.loads(req.data) == {"devices": [{"id": "d1", "actions": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "value": True}}]}]}


def test_device_error_is_raised(monkeypatch):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen({"status": "ok", "devices": [{"id": "d1", "capabilities": [
        {"type": "devices.capabilities.on_off", "state": {"instance": "on", "action_result": {
            "status": "ERROR", "error_code": "DEVICE_UNREACHABLE", "error_message": "Устройство не в сети"}}}]}]}))
    with pytest.raises(yandex.YandexError, match="Устройство не в сети"):
        yandex.set_on("tok", "d1", True)


@pytest.mark.parametrize("code", [401, 403])
def test_auth_errors(monkeypatch, code):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=http_error(code)))
    with pytest.raises(yandex.YandexAuthError):
        yandex.list_devices("tok")


def test_network_and_server_errors(monkeypatch):
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=http_error(500)))
    with pytest.raises(yandex.YandexError) as e:
        yandex.list_devices("tok")
    assert not isinstance(e.value, yandex.YandexAuthError)
    monkeypatch.setattr(yandex.urllib.request, "urlopen", FakeUrlopen(error=OSError("timeout")))
    with pytest.raises(yandex.YandexError, match="недоступен"):
        yandex.set_on("tok", "d1", False)
```

- [ ] **Step 3: Run to verify failure** — `cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q tests/test_secret_box.py tests/test_yandex.py`. Expected: FAIL — `ModuleNotFoundError: app.services.secret_box`.

- [ ] **Step 4: Implement**

`backend/app/services/secret_box.py`:

```python
"""Шифрование секретов учётки (токен Яндекса) ключом, выведенным из JWT_SECRET.
Сменили JWT_SECRET — старые токены не расшифруются, их вводят заново."""

import base64

from cryptography.fernet import Fernet, InvalidToken
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.hkdf import HKDF


def _fernet(secret: str) -> Fernet:
    key = HKDF(algorithm=hashes.SHA256(), length=32, salt=None, info=b"poliv:yandex-token").derive(secret.encode())
    return Fernet(base64.urlsafe_b64encode(key))


def encrypt(plain: str, secret: str) -> str:
    return _fernet(secret).encrypt(plain.encode()).decode()


def decrypt(token: str, secret: str) -> str | None:
    try:
        return _fernet(secret).decrypt(token.encode()).decode()
    except InvalidToken:
        return None
```

`backend/app/services/yandex.py`:

```python
"""Умный дом Яндекса: список розеток и вкл/выкл. Токен — OAuth пользователя с правами
iot:view и iot:control. Документация: yandex.ru/dev/dialogs/smart-home/doc/ru/concepts/platform-protocol"""

import json
import urllib.error
import urllib.request
from dataclasses import dataclass

API = "https://api.iot.yandex.net/v1.0"
TIMEOUT = 10
ON_OFF = "devices.capabilities.on_off"


class YandexError(Exception):
    """Яндекс недоступен или устройство не выполнило команду."""


class YandexAuthError(YandexError):
    """Токен не принят (401/403): отозван, истёк или без нужных прав."""


@dataclass(frozen=True)
class Device:
    id: str
    name: str
    room: str | None
    type: str


def _call(token: str, method: str, path: str, body: dict | None = None) -> dict:
    req = urllib.request.Request(
        f"{API}{path}",
        data=None if body is None else json.dumps(body).encode(),
        method=method,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            payload = json.load(resp)
    except urllib.error.HTTPError as e:  # раньше OSError: HTTPError — его подкласс
        if e.code in (401, 403):
            raise YandexAuthError("Яндекс не принял токен") from e
        raise YandexError(f"Яндекс ответил ошибкой {e.code}") from e
    except (OSError, ValueError) as e:
        raise YandexError("Умный дом Яндекса недоступен") from e
    if payload.get("status") != "ok":
        raise YandexError(payload.get("message") or "Яндекс вернул ошибку")
    return payload


def list_devices(token: str) -> list[Device]:
    data = _call(token, "GET", "/user/info")
    rooms = {r["id"]: r["name"] for r in data.get("rooms", [])}
    return [
        Device(d["id"], d["name"], rooms.get(d.get("room")), d.get("type", ""))
        for d in data.get("devices", [])
        if any(c.get("type") == ON_OFF for c in d.get("capabilities", []))
    ]


def set_on(token: str, device_id: str, on: bool) -> None:
    data = _call(
        token,
        "POST",
        "/devices/actions",
        {"devices": [{"id": device_id, "actions": [{"type": ON_OFF, "state": {"instance": "on", "value": on}}]}]},
    )
    for device in data.get("devices", []):
        for cap in device.get("capabilities", []):
            result = cap.get("state", {}).get("action_result", {})
            if result.get("status") == "ERROR":
                raise YandexError(result.get("error_message") or result.get("error_code") or "Розетка не выполнила команду")
```

- [ ] **Step 5: Run to verify pass** — та же команда. Expected: PASS (все тесты: также `pytest -q` целиком проходит).

- [ ] **Step 6: Commit**

```bash
git add backend/requirements.txt backend/app/services/secret_box.py backend/app/services/yandex.py backend/tests/test_secret_box.py backend/tests/test_yandex.py
git commit -m "Лампы: клиент Умного дома Яндекса и шифрование токена"
```

---

### Task 3: Модели и миграция 0004

**Files:**
- Modify: `backend/app/models.py`
- Create: `backend/alembic/versions/0004_lamps.py`
- Test: create `backend/tests/test_migration.py`

**Interfaces:**
- Produces (models):
  - `LampMode(str, Enum)`: `auto | schedule | manual`; `LampSource(str, Enum)`: `manual | schedule | auto`
  - `Lamp`: `id, user_id, name, mode, device_id, device_name, morning_not_before: time, evening_not_after: time, last_state: bool | None, last_error, last_error_at, archived_at`
  - `PlantLamp`: `id, plant_id, lamp_id, started_at, ended_at`
  - `LampSession.lamp_id: int`, `LampSession.source: LampSource` (поле `plant_id` удалено)
  - `LampSchedule.lamp_id: int` (поле `plant_id` удалено)
  - `UserSettings.yandex_token: str | None`, `UserSettings.yandex_token_invalid: bool`, свойство `UserSettings.yandex_status -> "none" | "ok" | "invalid"`
- Индексы только в миграции (в моделях их нет — autogenerate может предложить удалить, не соглашаться): `uq_lamp_one_open ON lamp_sessions (lamp_id) WHERE ended_at IS NULL`, `uq_plant_lamp_open ON plant_lamps (plant_id) WHERE ended_at IS NULL`.

Примечание: после этой задачи код приложения (сервисы/роутеры) временно не импортируется — он чинится в Task 4–6. Тест миграции работает только с миграциями и SQL, поэтому запускается изолированно (`-k migration`, файл импортирует `app.db`, а не сервисы).

- [ ] **Step 1: Write the failing test** — `backend/tests/test_migration.py`:

```python
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
```

- [ ] **Step 2: Run to verify failure** (команда Postgres из шапки, с `python -m pytest -q tests/test_migration.py`). Expected: FAIL — `upgrade head` доходит только до 0003, запрос к `lamps` падает с `relation "lamps" does not exist`.

- [ ] **Step 3: Update models** — в `models.py`:

После `FeedMethod`:

```python
class LampMode(str, enum.Enum):
    auto = "auto"  # досвечивать до нормы через розетку
    schedule = "schedule"  # по интервалам (программируемая или умная розетка)
    manual = "manual"  # только кнопкой


class LampSource(str, enum.Enum):
    manual = "manual"
    schedule = "schedule"
    auto = "auto"
```

Перед `class LampSession` добавить:

```python
class Lamp(Base):
    """Лампа учётки. Растения под ней — периоды plant_lamps; розетка — устройство в Умном доме Яндекса."""

    __tablename__ = "lamps"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[int] = _owner()
    name: Mapped[str] = mapped_column(String(100))
    mode: Mapped[LampMode] = mapped_column(_enum(LampMode, "lamp_mode"), default=LampMode.manual)
    # NULL — розетки нет или она не умная: лампа только считает часы
    device_id: Mapped[str | None] = mapped_column(String(100))
    device_name: Mapped[str | None] = mapped_column(String(200))
    morning_not_before: Mapped[time] = mapped_column(Time, default=time(6))
    evening_not_after: Mapped[time] = mapped_column(Time, default=time(23))
    # Что последним отправили в розетку; команда уходит только при смене нужного состояния
    last_state: Mapped[bool | None] = mapped_column(Boolean)
    last_error: Mapped[str | None] = mapped_column(Text)
    last_error_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    # Не NULL — лампу «удалили»: скрыта, сессии остаются для истории растений
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))

    __table_args__ = (CheckConstraint("evening_not_after > morning_not_before", name="lamp_bounds_order"),)


class PlantLamp(Base):
    """Период, когда растение стояло под лампой. ended_at NULL — стоит сейчас; открытый период
    у растения один (частичный unique-индекс uq_plant_lamp_open в миграции 0004)."""

    __tablename__ = "plant_lamps"

    id: Mapped[int] = mapped_column(primary_key=True)
    plant_id: Mapped[int] = mapped_column(ForeignKey("plants.id", ondelete="CASCADE"), index=True)
    lamp_id: Mapped[int] = mapped_column(ForeignKey("lamps.id", ondelete="CASCADE"), index=True)
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    ended_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))

    __table_args__ = (CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="plant_lamp_end_after_start"),)
```

В `LampSession` заменить поле `plant_id` (с комментарием) на:

```python
    lamp_id: Mapped[int] = mapped_column(ForeignKey("lamps.id", ondelete="CASCADE"), index=True)
    source: Mapped[LampSource] = mapped_column(_enum(LampSource, "lamp_source"), default=LampSource.manual)
```

В `LampSchedule`: докстринг — `"""Интервал расписания лампы (местное время)."""`, поле `plant_id` заменить на:

```python
    lamp_id: Mapped[int] = mapped_column(ForeignKey("lamps.id", ondelete="CASCADE"), index=True)
```

В `UserSettings` в конец:

```python
    # Токен Умного дома Яндекса, зашифрован (services/secret_box.py)
    yandex_token: Mapped[str | None] = mapped_column(Text)
    yandex_token_invalid: Mapped[bool] = mapped_column(Boolean, default=False)

    @property
    def yandex_status(self) -> str:
        if self.yandex_token is None:
            return "none"
        return "invalid" if self.yandex_token_invalid else "ok"
```

- [ ] **Step 4: Write migration** — `backend/alembic/versions/0004_lamps.py`:

```python
"""Лампы как объект: lamps, периоды привязки растений, розетка в Умном доме Яндекса

- lamps: лампа учётки (режим auto/schedule/manual, устройство Яндекса, границы досветки, архив)
- plant_lamps: периоды «растение стояло под лампой»; открытый — не больше одного на растение
- lamp_sessions/lamp_schedules: plant_id → lamp_id; у сессий source (manual/schedule/auto)
- user_settings: зашифрованный токен Яндекса
Старые данные: общая лампа (plant_id NULL) и своя лампа растения становятся объектами-лампами.
Растение со своей лампой получает ещё и закрытый период общей — её часы в истории сохраняются.

Revision ID: 0004
Revises: 0003
Create Date: 2026-09-25
"""
import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision = "0004"
down_revision = "0003"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "lamps",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("user_id", sa.Integer, sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("name", sa.String(100), nullable=False),
        sa.Column("mode", sa.Enum("auto", "schedule", "manual", name="lamp_mode"), nullable=False, server_default="manual"),
        sa.Column("device_id", sa.String(100)),
        sa.Column("device_name", sa.String(200)),
        sa.Column("morning_not_before", sa.Time, nullable=False, server_default="06:00"),
        sa.Column("evening_not_after", sa.Time, nullable=False, server_default="23:00"),
        sa.Column("last_state", sa.Boolean),
        sa.Column("last_error", sa.Text),
        sa.Column("last_error_at", sa.DateTime(timezone=True)),
        sa.Column("archived_at", sa.DateTime(timezone=True)),
        sa.Column("tmp_plant_id", sa.Integer),  # только на время переноса данных
        sa.CheckConstraint("evening_not_after > morning_not_before", name="lamp_bounds_order"),
    )
    op.create_table(
        "plant_lamps",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("lamp_id", sa.Integer, sa.ForeignKey("lamps.id", ondelete="CASCADE"), nullable=False, index=True),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False, server_default=sa.func.now()),
        sa.Column("ended_at", sa.DateTime(timezone=True)),
        sa.CheckConstraint("ended_at IS NULL OR ended_at >= started_at", name="plant_lamp_end_after_start"),
    )
    op.execute("CREATE UNIQUE INDEX uq_plant_lamp_open ON plant_lamps (plant_id) WHERE ended_at IS NULL")

    op.execute("CREATE TYPE lamp_source AS ENUM ('manual', 'schedule', 'auto')")
    op.add_column("lamp_sessions", sa.Column("lamp_id", sa.Integer))
    op.add_column(
        "lamp_sessions",
        sa.Column("source", postgresql.ENUM(name="lamp_source", create_type=False), nullable=False, server_default="manual"),
    )
    op.add_column("lamp_schedules", sa.Column("lamp_id", sa.Integer))

    # --- перенос данных ---
    has_schedule = "EXISTS (SELECT 1 FROM lamp_schedules s WHERE {cond})"
    op.execute(
        f"""
        INSERT INTO lamps (user_id, name, mode)
        SELECT u.user_id, 'Общая лампа',
               CASE WHEN {has_schedule.format(cond="s.user_id = u.user_id AND s.plant_id IS NULL")}
                    THEN 'schedule' ELSE 'manual' END::lamp_mode
        FROM (SELECT user_id FROM lamp_sessions WHERE plant_id IS NULL
              UNION SELECT user_id FROM lamp_schedules WHERE plant_id IS NULL) u
        """
    )
    op.execute(
        f"""
        INSERT INTO lamps (user_id, name, mode, tmp_plant_id)
        SELECT p.user_id, left('Лампа ' || p.name, 100),
               CASE WHEN {has_schedule.format(cond="s.plant_id = p.id")} THEN 'schedule' ELSE 'manual' END::lamp_mode,
               p.id
        FROM plants p
        WHERE EXISTS (SELECT 1 FROM lamp_sessions x WHERE x.plant_id = p.id)
           OR EXISTS (SELECT 1 FROM lamp_schedules x WHERE x.plant_id = p.id)
        """
    )
    for table in ("lamp_sessions", "lamp_schedules"):
        op.execute(f"UPDATE {table} t SET lamp_id = l.id FROM lamps l WHERE l.tmp_plant_id = t.plant_id")
        op.execute(
            f"UPDATE {table} t SET lamp_id = l.id FROM lamps l "
            "WHERE t.plant_id IS NULL AND l.tmp_plant_id IS NULL AND l.user_id = t.user_id"
        )
    op.execute(
        "UPDATE lamp_sessions SET source = CASE WHEN schedule_id IS NOT NULL THEN 'schedule' ELSE 'manual' END::lamp_source"
    )
    # Период начинается с появления растения, но не позже первой сессии лампы — история целиком
    first = "LEAST(p.added_at, COALESCE((SELECT min(s.started_at) FROM lamp_sessions s WHERE s.lamp_id = l.id), p.added_at))"
    op.execute(
        f"INSERT INTO plant_lamps (plant_id, lamp_id, started_at) "
        f"SELECT p.id, l.id, {first} FROM lamps l JOIN plants p ON p.id = l.tmp_plant_id"
    )
    op.execute(
        f"""
        INSERT INTO plant_lamps (plant_id, lamp_id, started_at, ended_at)
        SELECT p.id, l.id, {first},
               CASE WHEN EXISTS (SELECT 1 FROM lamps o WHERE o.tmp_plant_id = p.id) THEN now() END
        FROM plants p JOIN lamps l ON l.user_id = p.user_id AND l.tmp_plant_id IS NULL
        """
    )

    # --- новая схема сессий и расписаний ---
    op.execute("DROP INDEX uq_lamp_one_open")
    for table in ("lamp_sessions", "lamp_schedules"):
        op.drop_column(table, "plant_id")
        op.alter_column(table, "lamp_id", nullable=False)
        op.create_foreign_key(f"{table}_lamp_id_fkey", table, "lamps", ["lamp_id"], ["id"], ondelete="CASCADE")
        op.create_index(f"ix_{table}_lamp_id", table, ["lamp_id"])
    op.execute("CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (lamp_id) WHERE ended_at IS NULL")
    op.drop_column("lamps", "tmp_plant_id")

    op.add_column("user_settings", sa.Column("yandex_token", sa.Text))
    op.add_column("user_settings", sa.Column("yandex_token_invalid", sa.Boolean, nullable=False, server_default=sa.false()))


def downgrade() -> None:
    op.drop_column("user_settings", "yandex_token_invalid")
    op.drop_column("user_settings", "yandex_token")

    for table in ("lamp_sessions", "lamp_schedules"):
        op.add_column(table, sa.Column("plant_id", sa.Integer, sa.ForeignKey("plants.id", ondelete="CASCADE"), index=True))
    # «Лампа <имя>» с одним растением снова своя лампа растения, остальные — общая
    op.execute(
        """
        CREATE TEMP TABLE own_lamp AS
        SELECT l.id AS lamp_id, min(pl.plant_id) AS plant_id
        FROM lamps l JOIN plant_lamps pl ON pl.lamp_id = l.id AND pl.ended_at IS NULL
        WHERE l.name LIKE 'Лампа %' GROUP BY l.id HAVING count(*) = 1
        """
    )
    for table in ("lamp_sessions", "lamp_schedules"):
        op.execute(f"UPDATE {table} t SET plant_id = o.plant_id FROM own_lamp o WHERE o.lamp_id = t.lamp_id")
    op.execute("DROP TABLE own_lamp")

    op.execute("DROP INDEX uq_lamp_one_open")
    op.execute("UPDATE lamp_sessions SET ended_at = now() WHERE ended_at IS NULL")  # старый индекс — одна на учётку
    op.execute("CREATE UNIQUE INDEX uq_lamp_one_open ON lamp_sessions (user_id, COALESCE(plant_id, 0)) WHERE ended_at IS NULL")
    op.drop_column("lamp_sessions", "source")
    op.execute("DROP TYPE lamp_source")
    for table in ("lamp_sessions", "lamp_schedules"):
        op.drop_column(table, "lamp_id")
    op.drop_table("plant_lamps")
    op.drop_table("lamps")
    op.execute("DROP TYPE lamp_mode")
```

- [ ] **Step 5: Run to verify pass** — команда Postgres из шапки с `python -m pytest -q tests/test_migration.py`. Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/app/models.py backend/alembic/versions/0004_lamps.py backend/tests/test_migration.py
git commit -m "Лампы: модели и миграция 0004 — лампы, периоды привязки, токен Яндекса"
```

---

### Task 4: Схемы, crud и сервис ламп (привязка, сессии растения, переключение)

**Files:**
- Modify: `backend/app/schemas.py`, `backend/app/crud.py`, `backend/app/services/light.py`
- Create: `backend/app/services/lamps.py` (первая часть)

**Interfaces:**
- Consumes: Task 1 (`rules.clip_session`), Task 3 (модели).
- Produces:
  - schemas: `LampFields`, `LampCreate(+plant_ids)`, `LampUpdate`, `PlannedInterval(start, end)`, `LampOut`, `LampBrief(id, name, mode, is_on, has_device, planned, last_error)`, `PlantLampSet(lamp_id)`, `YandexTokenSet(token)`, `YandexDevice`, `LampScheduleSet(intervals)`; `LampSessionCreate.lamp_id: int`; `LampSessionOut(lamp_id, source)`; `LampToggle(plant_id: int)`; `LampToggleOut(+plug_error)`; `SettingsOut.yandex_status`; `HistoryEvent.lamp_name` вместо `shared`; `LampSummary` без `shared_is_on`; `LightSummary.lamp: LampBrief | None` вместо `schedule`/`shared_schedule`.
  - crud: `owned_lamp(db, user, lamp_id) -> Lamp` (архивная → 404), `owned_lamp_session(db, user, session_id) -> LampSession` (переименование старого `owned_lamp`).
  - light: `schedules_for(db, lamp_id) -> list[LampSchedule]`, `ensure_schedule_sessions(db, user_id, day) -> int`, `replace_schedule(db, user_id, lamp_id, intervals) -> list[LampSchedule]`, `validate_intervals(intervals: list[ScheduleInterval]) -> list[tuple[time, time]]` (400 при ошибке), `sync_all(db)` — только свет.
  - lamps: `current_lamp_id(db, plant_id) -> int | None`, `plant_ids(db, lamp_id) -> list[int]`, `assign(db, plant_id, lamp_id | None, now)`, `set_plants(db, lamp, ids, now)`, `PlantSession(id, lamp_id, lamp_name, started_at, ended_at)`, `plant_sessions(db, plant_id, since=None) -> list[PlantSession]`, `as_rules(list[PlantSession]) -> list[rules.Session]`, `covering_session(db, lamp_id, now) -> LampSession | None`.

Тестов в этой задаче нет отдельно: сервис проверяется API-тестами Task 6 (там же — изоляция). Проверка задачи — импорт и юнит-тесты.

- [ ] **Step 1: Schemas** — в `schemas.py`:
  - импорт: `from app.models import FeedMethod, LampMode, LampSource, Season`
  - заменить блок лампы (`LampSessionCreate` … `LampScheduleSet`) на:

```python
class LampSessionCreate(BaseModel):
    lamp_id: int
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionUpdate(BaseModel):
    started_at: datetime | None = None
    ended_at: datetime | None = None
    planned_hours_per_day: float | None = Field(None, ge=0, le=24)


class LampSessionOut(ORM):
    id: int
    lamp_id: int
    source: LampSource
    started_at: datetime
    ended_at: datetime | None
    planned_hours_per_day: float


class LampToggle(BaseModel):
    plant_id: int


class LampToggleOut(BaseModel):
    is_on: bool
    session: LampSessionOut
    # Для отмены выключения: вернуть ended_at к этому значению (null — горела вручную)
    previous_ended_at: datetime | None = None
    # Сессия записана, но розетка не ответила — текст для тоста
    plug_error: str | None = None


class ScheduleInterval(BaseModel):
    start_time: time
    end_time: time


class LampScheduleSet(BaseModel):
    """Полная замена расписания лампы; пустой список — расписания нет."""

    intervals: list[ScheduleInterval] = Field(default_factory=list, max_length=8)


# ---------- Лампы ----------
class LampFields(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    mode: LampMode = LampMode.manual
    device_id: str | None = Field(None, max_length=100)
    device_name: str | None = Field(None, max_length=200)
    morning_not_before: time = time(6)
    evening_not_after: time = time(23)


class LampCreate(LampFields):
    plant_ids: list[int] = Field(default_factory=list, max_length=100)


class LampUpdate(BaseModel):
    name: str | None = Field(None, min_length=1, max_length=100)
    mode: LampMode | None = None
    device_id: str | None = Field(None, max_length=100)
    device_name: str | None = Field(None, max_length=200)
    morning_not_before: time | None = None
    evening_not_after: time | None = None
    plant_ids: list[int] | None = Field(None, max_length=100)


class PlannedInterval(BaseModel):
    start: datetime
    end: datetime | None


class LampOut(LampFields):
    id: int
    last_state: bool | None
    last_error: str | None
    last_error_at: datetime | None
    plant_ids: list[int]
    is_on: bool
    schedule: list[ScheduleInterval]
    planned: list[PlannedInterval]  # досветка на сегодня (режим auto)


class LampBrief(BaseModel):
    """Лампа растения в сводке."""

    id: int
    name: str
    mode: LampMode
    is_on: bool
    has_device: bool
    planned: list[PlannedInterval]
    last_error: str | None


class PlantLampSet(BaseModel):
    lamp_id: int | None


class YandexTokenSet(BaseModel):
    token: str | None = Field(None, min_length=10, max_length=2000)


class YandexDevice(BaseModel):
    id: str
    name: str
    room: str | None
    type: str
```

  - `SettingsOut`: добавить `yandex_status: Literal["none", "ok", "invalid"]`.
  - `LampSummary`: удалить поле `shared_is_on`.
  - `LightSummary`: удалить `schedule` и `shared_schedule`, добавить `lamp: LampBrief | None  # лампа растения сейчас`.
  - `HistoryEvent`: `shared: bool | None = None` заменить на `lamp_name: str | None = None`.

- [ ] **Step 2: crud** — в `crud.py` импорт `from app.models import FertilizerType, Lamp, LampSession, Plant, User`; старую `owned_lamp` переименовать в `owned_lamp_session`; добавить:

```python
def owned_lamp(db: Session, user: User, lamp_id: int) -> Lamp:
    """Архивная («удалённая») лампа тоже 404."""
    obj = db.get(Lamp, lamp_id)
    if obj is None or obj.user_id != user.id or obj.archived_at is not None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "Лампа не найдена")
    return obj
```

- [ ] **Step 3: light.py — расписания по лампе** — заменить блок «расписание → сессии» и `sync_all`:

```python
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
```

  Импорты в `light.py`: `from datetime import date, datetime, time, timedelta`; `from fastapi import HTTPException, status`; `from app import schemas`; `from app.models import DaylightDay, Lamp, LampMode, LampSchedule, LampSession, LampSource, User, UserSettings`; функцию `_lamp_cond` удалить. Докстринг модуля: «…и сессии лампы по расписанию».

- [ ] **Step 4: lamps.py — первая часть** — `backend/app/services/lamps.py`:

```python
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
```

- [ ] **Step 5: Verify** — `cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt python -c "import app.services.lamps, app.services.light"` (env: `DATABASE_URL=postgresql+psycopg://x/y JWT_SECRET=x` перед командой). Expected: без ошибок. Роутеры/`plants.py` пока сломаны — чинятся в Task 5–6.

- [ ] **Step 6: Commit**

```bash
git add backend/app/schemas.py backend/app/crud.py backend/app/services/light.py backend/app/services/lamps.py
git commit -m "Лампы: схемы, владение, привязка растений и их сессии по периодам"
```

---

### Task 5: Досветка, розетка, архив и шаг раз в минуту

**Files:**
- Modify: `backend/app/services/lamps.py` (вторая часть), `backend/app/main.py`

**Interfaces:**
- Consumes: Task 1 (`plan_morning`, `plan_evening`, `lamp_need`, `light_state`, `lamp_hours_in_day`), Task 2 (`yandex`, `secret_box`), Task 4.
- Produces (в `lamps.py`):
  - `Toggled(is_on: bool, session: LampSession, previous_ended_at: datetime | None)`; `toggle(db, lamp, now) -> Toggled` (commit + `sync_plug`)
  - `replan_auto(db, lamp, now) -> None`; `stop_auto(db, lamp, now) -> None`
  - `yandex_token(db, user_id) -> str | None`; `sync_plug(db, lamp, now) -> None`
  - `update(db, lamp, data: dict, ids: list[int] | None, now) -> None`; `move_plant(db, plant_id, lamp_id | None, now) -> None`; `archive(db, lamp, now) -> None`
  - `lamp_out(db, lamp, now) -> schemas.LampOut`; `brief(db, lamp, now) -> schemas.LampBrief`
  - `tick(db, now: datetime | None = None) -> None`

Тесты этой задачи — в Task 6 (`test_auto_lamp_plans_and_drives_plug`, `test_plug_errors_and_archive`): им нужны роутеры. Здесь — только код и смоук-импорт.

- [ ] **Step 1: Implement** — дописать в `lamps.py`:

```python
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
NOON = time(12)


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
    """Досветка на сегодня: не начавшиеся части (утро до рассвета, вечер после заката) пересоздаются
    по свежим данным; начавшиеся и прошедшие не трогаются — в том числе выключенные кнопкой."""
    tz = settings.zone
    day = rules.local_date(now, tz)
    autos = _auto_today(db, lamp, now)
    for s in autos:
        if s.started_at > now:
            db.delete(s)
    db.flush()
    started = [s for s in autos if s.started_at <= now]
    noon = datetime.combine(day, NOON, tzinfo=tz)
    daylight = db.get(DaylightDay, (lamp.user_id, day))
    ids = plant_ids(db, lamp.id)
    if daylight is not None and ids:
        natural = daylight.sunshine_hours
        if not any(s.started_at < noon for s in started):
            need = _worst_deficit(db, ids, natural, now)
            _add_auto(db, lamp, rules.plan_morning(need, daylight.sunrise, lamp.morning_not_before, day, tz), now)
        if not any(s.started_at >= noon for s in started):
            remaining = _worst_deficit(db, ids, natural, now)  # утро уже учтено
            _add_auto(db, lamp, rules.plan_evening(remaining, daylight.sunset, lamp.evening_not_after, day, tz), now)
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
```

- [ ] **Step 2: main.py — второй фоновой цикл**:

```python
import asyncio
import logging
from collections.abc import Callable
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI

from app import auth
from app.db import SessionLocal
from app.routers import admin, fertilizers, lamp, lamps, light, logs, plants, settings
from app.services import lamps as lamps_svc
from app.services.light import sync_all

SYNC_EVERY_SECONDS = 30 * 60
TICK_EVERY_SECONDS = 60
log = logging.getLogger("poliv")


def _sync_once() -> None:
    with SessionLocal() as db:
        sync_all(db)


def _tick_once() -> None:
    with SessionLocal() as db:
        lamps_svc.tick(db)


async def _every(seconds: int, job: Callable[[], None], what: str) -> None:
    while True:
        try:
            await asyncio.to_thread(job)
        except Exception:
            log.exception("фоновая задача «%s» упала", what)
        await asyncio.sleep(seconds)


@asynccontextmanager
async def lifespan(_: FastAPI):
    """Фоном: раз в полчаса свет по городу учёток; раз в минуту — расписания, досветка и розетки."""
    tasks = [
        asyncio.create_task(_every(SYNC_EVERY_SECONDS, _sync_once, "свет по городу")),
        asyncio.create_task(_every(TICK_EVERY_SECONDS, _tick_once, "лампы и розетки")),
    ]
    yield
    for task in tasks:
        task.cancel()
```

  и в списке роутеров: `for r in (admin.router, plants.router, fertilizers.router, logs.router, lamp.router, lamps.router, lamps.yandex_router, light.router, settings.router):` (сам `routers/lamps.py` — в Task 6; до него `main.py` не импортируется — это ожидаемо).

- [ ] **Step 3: Verify** — `DATABASE_URL=postgresql+psycopg://x/y JWT_SECRET=x uv run --python 3.12 --with-requirements requirements-dev.txt python -c "import app.services.lamps"` из `backend/`. Expected: без ошибок; `pytest -q` (юнит) — PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/app/services/lamps.py backend/app/main.py
git commit -m "Лампы: досветка утро/вечер, управление розеткой, архив, шаг раз в минуту"
```

---

### Task 6: API ламп, сводка растения и тесты на реальной базе

**Files:**
- Create: `backend/app/routers/lamps.py`
- Modify: `backend/app/routers/lamp.py`, `backend/app/routers/light.py`, `backend/app/routers/plants.py`, `backend/app/routers/settings.py`, `backend/app/services/plants.py`
- Test: `backend/tests/test_api.py`

**Interfaces:**
- Consumes: Task 4–5.
- Produces (HTTP): `GET/POST /api/lamps`, `GET/PATCH/DELETE /api/lamps/{id}`, `PUT /api/lamps/{id}/schedule`, `POST /api/lamps/{id}/toggle`, `GET /api/yandex/devices`, `PUT /api/settings/yandex-token`, `PUT /api/plants/{id}/lamp`, `POST /api/lamp-sessions/toggle {plant_id}` (прокси на лампу растения), `GET /api/lamp-sessions?lamp_id=&open=`; удалены `GET/PUT /api/lamp-schedules`.

- [ ] **Step 1: Write the failing tests** — в `test_api.py`:

  (a) В `test_data_isolation` заменить блок «Общая лампа — своя у каждой учётки» (последние 5 строк до проверки `Фикус A`) на:

```python
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
```

  (b) Заменить `test_schedule_creates_sessions_and_toggle_ends_it` и `test_schedule_validation_and_isolation` на:

```python
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
        # лимону не хватает 10 ч: утро — половина, но не раньше 06:00 (3 ч); вечер — остаток 7 ч, но до 23:00
        assert planned(db) == [(at(6), at(9)), (at(16, 30), at(23))]
        assert calls == []  # розетка уже выключена

        lamps.tick(db, now=at(6, 30))
        assert calls == [("dev-1", True)]
        lamps.tick(db, now=at(6, 31))
        assert calls == [("dev-1", True)]  # команда только при смене состояния

        # Выключили кнопкой посреди утра — утро не создаётся заново
        lamps.toggle(db, db.get(Lamp, lamp_id), at(6, 40))
        assert calls[-1] == ("dev-1", False)
        lamps.tick(db, now=at(6, 41))
        assert calls[-1] == ("dev-1", False) and len(planned(db)) == 2

        # Смена режима во время вечерней досветки: идущая гаснет, розетке «выкл»
        lamps.tick(db, now=at(17))
        assert calls[-1] == ("dev-1", True)
        lamps.update(db, db.get(Lamp, lamp_id), {"mode": LampMode.manual}, None, at(17, 5))
        assert planned(db)[-1] == (at(16, 30), at(17, 5))
        assert calls[-1] == ("dev-1", False)


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
```

  Примечание к `test_plug_errors_and_archive`: `lamp2` создаётся при исправном `fake_set_on` — «выкл» проходит, `last_state=False`; `toggle` → нужно «вкл» → `auth_fail`.

- [ ] **Step 2: Run to verify failure** — команда Postgres из шапки. Expected: FAIL (`ImportError: cannot import name 'lamps' from app.routers` или 404 на `/api/lamps`).

- [ ] **Step 3: routers/lamps.py**:

```python
"""Лампы учётки: растения под лампой, режим, розетка, расписание, кнопка; устройства Яндекса."""

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import Lamp, LampMode, User
from app.services import lamps as svc
from app.services import light, yandex
from app.services.plants import get_user_settings, now_utc

router = APIRouter(prefix="/lamps", tags=["lamps"])
yandex_router = APIRouter(prefix="/yandex", tags=["lamps"])
DB = Annotated[Session, Depends(get_db)]


def _own_plant_ids(db: Session, user: User, ids: list[int]) -> list[int]:
    for pid in ids:
        crud.owned_plant(db, user, pid)
    return list(dict.fromkeys(ids))


def _check(db: Session, user: User, mode: LampMode, morning, evening) -> None:
    if evening <= morning:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "«Вечером не позже» должно быть позже «утром не раньше»")
    if mode == LampMode.auto and get_user_settings(db, user.id).latitude is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Для режима «Авто» задайте город в настройках (раздел «Свет»)")


def toggle_out(db: Session, lamp: Lamp) -> schemas.LampToggleOut:
    try:
        res = svc.toggle(db, lamp, now_utc())
    except IntegrityError:
        db.rollback()
        raise HTTPException(status.HTTP_409_CONFLICT, "Лампа уже горит, или время выключения раньше включения")
    return schemas.LampToggleOut(
        is_on=res.is_on,
        session=schemas.LampSessionOut.model_validate(res.session),
        previous_ended_at=res.previous_ended_at,
        plug_error=lamp.last_error,
    )


@router.get("", response_model=list[schemas.LampOut])
def list_lamps(user: CurrentUser, db: DB):
    now = now_utc()
    q = select(Lamp).where(Lamp.user_id == user.id, Lamp.archived_at.is_(None)).order_by(Lamp.id)
    return [svc.lamp_out(db, lamp, now) for lamp in db.scalars(q)]


@router.post("", response_model=schemas.LampOut, status_code=status.HTTP_201_CREATED)
def create_lamp(body: schemas.LampCreate, user: CurrentUser, db: DB):
    _check(db, user, body.mode, body.morning_not_before, body.evening_not_after)
    ids = _own_plant_ids(db, user, body.plant_ids)
    lamp = Lamp(user_id=user.id, **body.model_dump(exclude={"plant_ids"}))
    db.add(lamp)
    db.flush()
    now = now_utc()
    svc.set_plants(db, lamp, ids, now)
    db.commit()
    svc.after_change(db, lamp, now)
    return svc.lamp_out(db, lamp, now)


@router.get("/{lamp_id}", response_model=schemas.LampOut)
def get_lamp(lamp_id: int, user: CurrentUser, db: DB):
    return svc.lamp_out(db, crud.owned_lamp(db, user, lamp_id), now_utc())


@router.patch("/{lamp_id}", response_model=schemas.LampOut)
def update_lamp(lamp_id: int, body: schemas.LampUpdate, user: CurrentUser, db: DB):
    lamp = crud.owned_lamp(db, user, lamp_id)
    data = body.model_dump(exclude_unset=True)
    ids = data.pop("plant_ids", None)
    _check(
        db, user,
        data.get("mode", lamp.mode),
        data.get("morning_not_before", lamp.morning_not_before),
        data.get("evening_not_after", lamp.evening_not_after),
    )
    if ids is not None:
        ids = _own_plant_ids(db, user, ids)
    now = now_utc()
    svc.update(db, lamp, data, ids, now)
    return svc.lamp_out(db, lamp, now)


@router.delete("/{lamp_id}", status_code=status.HTTP_204_NO_CONTENT)
def archive_lamp(lamp_id: int, user: CurrentUser, db: DB):
    """Лампа уходит в архив: растения остаются без лампы, часы в их истории сохраняются."""
    svc.archive(db, crud.owned_lamp(db, user, lamp_id), now_utc())


@router.put("/{lamp_id}/schedule", response_model=list[schemas.ScheduleInterval])
def set_schedule(lamp_id: int, body: schemas.LampScheduleSet, user: CurrentUser, db: DB):
    """Полная замена расписания; пустой список — расписания нет."""
    lamp = crud.owned_lamp(db, user, lamp_id)
    intervals = light.validate_intervals(body.intervals)
    if lamp.mode != LampMode.schedule and intervals:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Расписание задаётся в режиме «По расписанию»")
    rows = light.replace_schedule(db, user.id, lamp.id, intervals)
    svc.sync_plug(db, lamp, now_utc())
    return [schemas.ScheduleInterval(start_time=r.start_time, end_time=r.end_time) for r in rows]


@router.post("/{lamp_id}/toggle", response_model=schemas.LampToggleOut)
def toggle(lamp_id: int, user: CurrentUser, db: DB):
    return toggle_out(db, crud.owned_lamp(db, user, lamp_id))


@yandex_router.get("/devices", response_model=list[schemas.YandexDevice])
def devices(user: CurrentUser, db: DB):
    """Устройства Умного дома, которые умеют вкл/выкл, — для выбора розетки лампы."""
    token = svc.yandex_token(db, user.id)
    if token is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Сначала вставьте токен Яндекса в разделе «Свет»")
    try:
        return [schemas.YandexDevice(id=d.id, name=d.name, room=d.room, type=d.type) for d in yandex.list_devices(token)]
    except yandex.YandexAuthError:
        get_user_settings(db, user.id).yandex_token_invalid = True
        db.commit()
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Токен Яндекса недействителен — вставьте новый в разделе «Свет»")
    except yandex.YandexError as e:
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, str(e))
```

- [ ] **Step 4: routers/lamp.py (сессии)**:

```python
from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app import crud, schemas
from app.auth import CurrentUser
from app.db import get_db
from app.models import Lamp, LampSession
from app.routers.lamps import toggle_out
from app.services import lamps as lamps_svc
from app.services.plants import now_utc

router = APIRouter(prefix="/lamp-sessions", tags=["lamp"])
DB = Annotated[Session, Depends(get_db)]


def _save(db: Session, obj: LampSession) -> LampSession:
    try:
        obj = crud.save(db, obj)
    except IntegrityError:
        db.rollback()
        raise HTTPException(status.HTTP_409_CONFLICT, "Лампа уже горит, или время выключения раньше включения")
    lamps_svc.sync_plug(db, db.get(Lamp, obj.lamp_id), now_utc())
    return obj


@router.get("", response_model=list[schemas.LampSessionOut])
def list_sessions(user: CurrentUser, db: DB, lamp_id: int | None = None, open: bool | None = None, limit: int = 100):
    """lamp_id — сессии одной лампы; open=true — горящие сейчас."""
    q = select(LampSession).where(LampSession.user_id == user.id).order_by(LampSession.started_at.desc()).limit(limit)
    if lamp_id is not None:
        q = q.where(LampSession.lamp_id == lamp_id)
    if open is not None:
        q = q.where(LampSession.ended_at.is_(None) if open else LampSession.ended_at.is_not(None))
    return db.scalars(q).all()


@router.post("", response_model=schemas.LampSessionOut, status_code=status.HTTP_201_CREATED)
def create_session(body: schemas.LampSessionCreate, user: CurrentUser, db: DB):
    lamp = crud.owned_lamp(db, user, body.lamp_id)
    session = LampSession(
        user_id=user.id, lamp_id=lamp.id, started_at=body.started_at or now_utc(), ended_at=body.ended_at
    )
    if body.planned_hours_per_day is not None:
        session.planned_hours_per_day = body.planned_hours_per_day
    return _save(db, session)


@router.post("/toggle", response_model=schemas.LampToggleOut)
def toggle(body: schemas.LampToggle, user: CurrentUser, db: DB):
    """Кнопка «Лампа» у растения — переключает его лампу (и соседей по ней)."""
    plant = crud.owned_plant(db, user, body.plant_id)
    lamp_id = lamps_svc.current_lamp_id(db, plant.id)
    if lamp_id is None:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "У растения нет лампы — привяжите её в настройках")
    return toggle_out(db, db.get(Lamp, lamp_id))


@router.get("/{session_id}", response_model=schemas.LampSessionOut)
def get_session(session_id: int, user: CurrentUser, db: DB):
    return crud.owned_lamp_session(db, user, session_id)


@router.patch("/{session_id}", response_model=schemas.LampSessionOut)
def update_session(session_id: int, body: schemas.LampSessionUpdate, user: CurrentUser, db: DB):
    """ended_at=null снова «зажигает» сессию — так работает отмена выключения."""
    obj = crud.owned_lamp_session(db, user, session_id)
    crud.apply_update(obj, body)
    return _save(db, obj)


@router.delete("/{session_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_session(session_id: int, user: CurrentUser, db: DB):
    obj = crud.owned_lamp_session(db, user, session_id)
    lamp = db.get(Lamp, obj.lamp_id)
    crud.delete(db, obj)
    lamps_svc.sync_plug(db, lamp, now_utc())
```

- [ ] **Step 5: routers/light.py** — удалить `list_schedules`, `set_schedule`, импорты `crud`, `select`, `LampSchedule`; докстринг: `"""Свет: поиск города и световой день сегодня."""`.

- [ ] **Step 6: routers/plants.py** — импорт `from app.services import lamps as lamps_svc`; в конец:

```python
@router.put("/{plant_id}/lamp", response_model=schemas.PlantLampSet)
def set_plant_lamp(plant_id: int, body: schemas.PlantLampSet, user: CurrentUser, db: DB):
    """Перенести растение под другую лампу (null — без лампы). Прошлые часы не меняются."""
    plant = crud.owned_plant(db, user, plant_id)
    if body.lamp_id is not None:
        crud.owned_lamp(db, user, body.lamp_id)
    lamps_svc.move_plant(db, plant.id, body.lamp_id, svc.now_utc())
    return body
```

- [ ] **Step 7: routers/settings.py** — импорты `from fastapi import APIRouter, Depends, HTTPException, status`, `from app.config import settings as config`, `from app.services import light, secret_box, yandex`; в конец:

```python
@router.put("/yandex-token", response_model=schemas.SettingsOut)
def set_yandex_token(body: schemas.YandexTokenSet, user: CurrentUser, db: DB):
    """Сохранить токен Умного дома (сразу проверяется у Яндекса) или удалить (null). Токен не возвращается."""
    row = get_user_settings(db, user.id)
    if body.token is None:
        row.yandex_token, row.yandex_token_invalid = None, False
        return crud.save(db, row)
    token = body.token.strip()
    try:
        yandex.list_devices(token)
    except yandex.YandexAuthError:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "Яндекс не принял токен — проверьте права «Умный дом» (iot:view, iot:control)")
    except yandex.YandexError as e:
        raise HTTPException(status.HTTP_502_BAD_GATEWAY, str(e))
    row.yandex_token = secret_box.encrypt(token, config.jwt_secret)
    row.yandex_token_invalid = False
    return crud.save(db, row)
```

- [ ] **Step 8: services/plants.py** — сводка, история, статистика через лампы:
  - импорты: убрать `or_`, `LampSession`; добавить `Lamp`; `from app.services import lamps as lamps_svc`.
  - удалить `_lamp_filter`, `_lamp_sessions`, `covering_session`, `_intervals`.
  - `_light`: сигнатура `_light(plant: Plant, app: UserSettings, daylight: DaylightDay | None, sessions: list[rules.Session], now: datetime, lamp: schemas.LampBrief | None) -> schemas.LightSummary`; в теле убрать `schedule=`/`shared_schedule=`, добавить `lamp=lamp`.
  - в `build_summary` заменить блок от `midnight = …` до `own = …` и `lamp=`/`light=` в ответе:

```python
    midnight = rules.local_midnight(rules.local_date(now, tz), tz)
    sessions = lamps_svc.as_rules(lamps_svc.plant_sessions(db, plant.id, midnight))
    hours = rules.lamp_hours_today(sessions, now, tz)
    lamp_id = lamps_svc.current_lamp_id(db, plant.id)
    lamp = db.get(Lamp, lamp_id) if lamp_id is not None else None
    current = lamps_svc.covering_session(db, lamp_id, now) if lamp_id is not None else None
```

```python
        lamp=schemas.LampSummary(
            hours_today=round(hours, 2),
            planned_hours=plant.light_target_hours,
            status=rules.lamp_status(hours, plant.light_target_hours),
            is_on=current is not None,
            open_session_id=current.id if current else None,
        ),
        light=_light(plant, app, daylight, sessions, now, lamps_svc.brief(db, lamp, now) if lamp else None),
```

  - в `history` блок `if "lamp" in types:`:

```python
    if "lamp" in types:
        for s in lamps_svc.plant_sessions(db, plant_id, start):
            if (start is not None and s.started_at < start) or (end is not None and s.started_at >= end):
                continue
            hours = ((s.ended_at or now) - s.started_at).total_seconds() / 3600
            events.append(
                schemas.HistoryEvent(
                    type="lamp", id=s.id, at=s.started_at, ended_at=s.ended_at,
                    hours=round(hours, 2), lamp_name=s.lamp_name,
                )
            )
```

  - в `weekly`: `_lamp_sessions(db, plant, since)` → `lamps_svc.as_rules(lamps_svc.plant_sessions(db, plant_id, since))`.

- [ ] **Step 9: Run to verify pass** — команда Postgres из шапки (все тесты). Expected: PASS (включая юнит и `test_migration.py`).

- [ ] **Step 10: Commit**

```bash
git add backend/app/routers backend/app/services/plants.py backend/tests/test_api.py
git commit -m "Лампы: API ламп, токен и устройства Яндекса, сводка и история по периодам"
```

---

### Task 7: Фронтенд — типы, API и раздел «Лампы»

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/api.ts`, `frontend/src/components/LampScheduleEditor.tsx`, `frontend/src/components/LightSettings.tsx`, `frontend/src/pages/SettingsPage.tsx`, `frontend/src/styles.css`
- Create: `frontend/src/components/LampList.tsx`

**Interfaces:**
- Consumes: HTTP из Task 6.
- Produces (TS): `LampMode`, `LampFields`, `Lamp`, `LampBrief`, `PlannedInterval`, `YandexDevice`, `YandexStatus`, `LampToggle`; `api.lamps / createLamp / updateLamp / archiveLamp / setLampSchedule(lampId, …) / toggleLampById / setPlantLamp / toggleLamp(plantId) / deleteLampSession / restoreLampEnd / yandexDevices / setYandexToken`.

- [ ] **Step 1: types.ts**:
  - в `PlantSummary.lamp` удалить `shared_is_on`.
  - `AppSettings`: добавить `yandex_status: YandexStatus;`.
  - удалить `LampSchedule`; в `LightSummary` удалить `schedule`, `shared_schedule`, добавить `lamp: LampBrief | null;`.
  - `LampSession`: `plant_id` → `lamp_id: number; source: 'manual' | 'schedule' | 'auto';`.
  - `HistoryEvent`: `shared: boolean | null;` → `lamp_name: string | null;`.
  - добавить:

```ts
export type YandexStatus = 'none' | 'ok' | 'invalid';
export type LampMode = 'auto' | 'schedule' | 'manual';

/** Интервал досветки на сегодня */
export interface PlannedInterval {
  start: string;
  end: string | null;
}

export interface LampFields {
  name: string;
  mode: LampMode;
  device_id: string | null;
  device_name: string | null;
  /** 'HH:MM[:SS]' */
  morning_not_before: string;
  evening_not_after: string;
}

export interface Lamp extends LampFields {
  id: number;
  last_state: boolean | null;
  last_error: string | null;
  last_error_at: string | null;
  plant_ids: number[];
  is_on: boolean;
  schedule: ScheduleInterval[];
  planned: PlannedInterval[];
}

/** Лампа растения в сводке */
export interface LampBrief {
  id: number;
  name: string;
  mode: LampMode;
  is_on: boolean;
  has_device: boolean;
  planned: PlannedInterval[];
  last_error: string | null;
}

export interface LampToggle {
  is_on: boolean;
  session: LampSession;
  previous_ended_at: string | null;
  /** Сессия записана, но розетка не ответила */
  plug_error: string | null;
}

export interface YandexDevice {
  id: string;
  name: string;
  room: string | null;
  type: string;
}
```

- [ ] **Step 2: api.ts** — импорты типов: убрать `LampSchedule`, добавить `Lamp, LampFields, LampToggle, YandexDevice`; блок «Лампа» заменить на:

```ts
  // Лампы
  lamps: () => get<Lamp[]>('/lamps'),
  createLamp: (data: LampFields & { plant_ids: number[] }) => post<Lamp>('/lamps', data),
  updateLamp: (id: number, data: Partial<LampFields> & { plant_ids?: number[] }) => patch<Lamp>(`/lamps/${id}`, data),
  /** Лампа уходит в архив: растения без лампы, история часов сохраняется */
  archiveLamp: (id: number) => del(`/lamps/${id}`),
  setLampSchedule: (lampId: number, intervals: ScheduleInterval[]) =>
    request<ScheduleInterval[]>('PUT', `/lamps/${lampId}/schedule`, { intervals }),
  setPlantLamp: (plantId: number, lampId: number | null) =>
    request<{ lamp_id: number | null }>('PUT', `/plants/${plantId}/lamp`, { lamp_id: lampId }),
  // Кнопка гасит то, что горит (вручную, по расписанию, досветка), иначе включает вручную
  toggleLamp: (plantId: number) => post<LampToggle>('/lamp-sessions/toggle', { plant_id: plantId }),
  toggleLampById: (lampId: number) => post<LampToggle>(`/lamps/${lampId}/toggle`),
  /** Отмена выключения: вернуть прежний конец (null — снова горит вручную) */
  restoreLampEnd: (id: number, endedAt: string | null) => patch<LampSession>(`/lamp-sessions/${id}`, { ended_at: endedAt }),
  deleteLampSession: (id: number) => del(`/lamp-sessions/${id}`),

  // Умный дом Яндекса
  yandexDevices: () => get<YandexDevice[]>('/yandex/devices'),
  /** null — удалить токен */
  setYandexToken: (token: string | null) => request<AppSettings>('PUT', '/settings/yandex-token', { token }),
```

- [ ] **Step 3: LampScheduleEditor.tsx** — проп `plantId: number | null` → `lampId: number`; докстринг «Расписание лампы. Сохраняется сразу, отдельно от формы.»; `api.setLampSchedule(lampId, …)`; в `useEffect` зависимость `[lampId, JSON.stringify(initial)]`.

- [ ] **Step 4: LightSettings.tsx** — проп только `initial: AppSettings`; удалить импорт `LampScheduleEditor`, `LampSchedule` и блок «Общая лампа — расписание розетки»; импорт типа `YandexStatus`; вместо удалённого блока — `<YandexToken status={settings.yandex_status} onChange={setSettings} />`; докстринг компонента: «Город для светового дня и токен Умного дома Яндекса. Сохраняется сразу.». Компонент в том же файле:

```tsx
const TOKEN_HINT: Record<YandexStatus, string> = {
  none: 'Не подключено — лампы только считают часы, розетками не управляют',
  ok: 'Подключено — Поливалка включает и выключает розетки ламп',
  invalid: 'Яндекс больше не принимает токен — вставьте новый',
};

/** Токен Умного дома: проверяется у Яндекса при сохранении, обратно не показывается */
function YandexToken({ status, onChange }: { status: YandexStatus; onChange: (s: AppSettings) => void }) {
  const toast = useToast();
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);

  async function save(value: string | null) {
    setBusy(true);
    try {
      onChange(await api.setYandexToken(value));
      setToken('');
      toast(value ? 'Умный дом Яндекса подключён' : 'Токен Яндекса удалён');
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить токен');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="field field--stack">
      <span className="field__label">Умный дом Яндекса</span>
      <span className="field__hint">
        {TOKEN_HINT[status]}. Токен — на{' '}
        <a href="https://oauth.yandex.ru/client/new" target="_blank" rel="noreferrer">oauth.yandex.ru</a>{' '}
        (права «Умный дом»: просмотр и управление), пошагово — в инструкции.
      </span>
      <div className="input-row" onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); if (token.trim().length >= 10) save(token.trim()); } }}>
        <input
          className="input"
          type="password"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={status === 'none' ? 'y0_…' : 'Новый токен'}
          aria-label="Токен Яндекса"
        />
        <button className="btn btn--sm" type="button" disabled={busy || token.trim().length < 10} onClick={() => save(token.trim())}>
          Проверить
        </button>
      </div>
      {status !== 'none' && (
        <div className="btn-row" style={{ marginTop: 8 }}>
          <button className="btn btn--sm btn--danger" type="button" disabled={busy} onClick={() => save(null)}>Отключить</button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 5: LampList.tsx**:

```tsx
import { useState } from 'react';
import { api } from '../api';
import type { Lamp, LampFields, LampMode, Plant } from '../types';
import { useAsync } from '../useAsync';
import { Segmented } from './controls';
import { LampScheduleEditor } from './LampScheduleEditor';
import { useToast } from './toast';

const MODES: { value: LampMode; label: string }[] = [
  { value: 'auto', label: 'Авто' },
  { value: 'schedule', label: 'По расписанию' },
  { value: 'manual', label: 'Вручную' },
];

const MODE_HINT: Record<LampMode, string> = {
  auto: 'Досвечивает до нормы самого требовательного растения: половину утром до рассвета, остаток вечером после заката. Нужны город и розетка.',
  schedule: 'Горит по интервалам — как программируемая розетка.',
  manual: 'Только кнопкой «Лампа».',
};

const hhmm = (t: string) => t.slice(0, 5);

const EMPTY: LampFields = {
  name: '',
  mode: 'manual',
  device_id: null,
  device_name: null,
  morning_not_before: '06:00',
  evening_not_after: '23:00',
};

function meta(l: Lamp, plants: Plant[]): string {
  const names = plants.filter((p) => l.plant_ids.includes(p.id)).map((p) => p.name);
  return [
    MODES.find((m) => m.value === l.mode)?.label,
    l.device_name ? `розетка «${l.device_name}»` : 'без розетки',
    names.length ? names.join(', ') : 'растений нет',
    l.is_on && 'горит',
  ].filter(Boolean).join(' · ');
}

/** Лампы учётки: растения под лампой, режим и розетка. Сохраняется сразу, отдельно от формы настроек. */
export function LampList({ lamps, plants, onChanged }: { lamps: Lamp[]; plants: Plant[]; onChanged: () => void }) {
  const toast = useToast();
  const [editing, setEditing] = useState<number | 'new' | null>(null);
  const done = () => { setEditing(null); onChanged(); };

  async function remove(l: Lamp) {
    if (!confirm(`Удалить «${l.name}»? Растения останутся без лампы, часы в истории сохранятся.`)) return;
    try {
      await api.archiveLamp(l.id);
      done();
    } catch {
      toast('Не удалось удалить');
    }
  }

  return (
    <div className="form-group">
      {lamps.length === 0 && editing !== 'new' && <p className="empty">Ламп пока нет</p>}
      {lamps.map((l) =>
        editing === l.id ? (
          <LampForm key={l.id} lamp={l} lamps={lamps} plants={plants} onCancel={() => setEditing(null)} onSaved={done} />
        ) : (
          <div className="fert-item" key={l.id}>
            <div className="fert-item__text">
              <div className="field__label">{l.name}</div>
              <div className="fert-item__meta">{meta(l, plants)}</div>
              {l.last_error && <div className="form-error">Розетка: {l.last_error}</div>}
            </div>
            <button className="btn btn--sm" type="button" onClick={() => setEditing(l.id)}>Изменить</button>
            <button className="btn btn--sm btn--danger" type="button" aria-label={`Удалить ${l.name}`} onClick={() => remove(l)}>✕</button>
          </div>
        ),
      )}
      {editing === 'new' ? (
        <LampForm lamps={lamps} plants={plants} onCancel={() => setEditing(null)} onSaved={done} />
      ) : (
        <div className="fert-item">
          <button className="btn btn--sm btn--ghost" type="button" onClick={() => setEditing('new')}>+ Добавить лампу</button>
        </div>
      )}
    </div>
  );
}

function LampForm({
  lamp,
  lamps,
  plants,
  onSaved,
  onCancel,
}: {
  lamp?: Lamp;
  lamps: Lamp[];
  plants: Plant[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [f, setF] = useState<LampFields>(
    lamp
      ? {
          name: lamp.name,
          mode: lamp.mode,
          device_id: lamp.device_id,
          device_name: lamp.device_name,
          morning_not_before: hhmm(lamp.morning_not_before),
          evening_not_after: hhmm(lamp.evening_not_after),
        }
      : EMPTY,
  );
  const [ids, setIds] = useState<number[]>(lamp?.plant_ids ?? []);
  const [error, setError] = useState<string | null>(null);
  // Без токена Яндекс ответит 400 с подсказкой — её и показываем
  const { data: devices, error: devicesError } = useAsync(() => api.yandexDevices(), []);

  const otherLamp = (plantId: number) => lamps.find((l) => l.id !== lamp?.id && l.plant_ids.includes(plantId));
  const toggleId = (plantId: number) => setIds(ids.includes(plantId) ? ids.filter((x) => x !== plantId) : [...ids, plantId]);

  function pickDevice(id: string) {
    const d = devices?.find((x) => x.id === id);
    setF({ ...f, device_id: d?.id ?? (id || null), device_name: d?.name ?? (id ? f.device_name : null) });
  }

  async function submit() {
    if (!f.name.trim()) return setError('Укажите название');
    try {
      const data = { ...f, name: f.name.trim(), plant_ids: ids };
      if (lamp) await api.updateLamp(lamp.id, data);
      else await api.createLamp(data);
      onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить');
    }
  }

  // Не <form>: блок живёт внутри формы настроек, вложенные формы запрещены
  return (
    <div className="fert-form">
      <label className="span-2">
        Название
        <input className="input" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Лампа цитрусы" />
      </label>
      <label className="span-2">
        Розетка
        <select className="select" value={f.device_id ?? ''} onChange={(e) => pickDevice(e.target.value)}>
          <option value="">Без розетки — только учёт часов</option>
          {f.device_id && !devices?.some((d) => d.id === f.device_id) && (
            <option value={f.device_id}>{f.device_name ?? f.device_id}</option>
          )}
          {devices?.map((d) => (
            <option key={d.id} value={d.id}>{d.name}{d.room ? ` — ${d.room}` : ''}</option>
          ))}
        </select>
      </label>
      {devicesError && <p className="field__hint span-2">{devicesError}</p>}
      <div className="span-2">
        <Segmented label="Режим" value={f.mode} options={MODES} onChange={(mode) => setF({ ...f, mode })} />
        <p className="field__hint" style={{ marginTop: 6 }}>{MODE_HINT[f.mode]}</p>
      </div>
      {f.mode === 'auto' && (
        <>
          <label>
            Утром не раньше
            <input className="input" type="time" value={f.morning_not_before} onChange={(e) => setF({ ...f, morning_not_before: e.target.value })} />
          </label>
          <label>
            Вечером не позже
            <input className="input" type="time" value={f.evening_not_after} onChange={(e) => setF({ ...f, evening_not_after: e.target.value })} />
          </label>
        </>
      )}
      {f.mode === 'schedule' &&
        (lamp?.mode === 'schedule' ? (
          <div className="span-2"><LampScheduleEditor lampId={lamp.id} initial={lamp.schedule} /></div>
        ) : (
          <p className="field__hint span-2">Сохраните лампу — после этого здесь появится расписание.</p>
        ))}
      <fieldset className="span-2 lamp-plants">
        <legend>Растения под лампой</legend>
        {plants.length === 0 && <p className="field__hint">Растений пока нет</p>}
        {plants.map((p) => {
          const other = otherLamp(p.id);
          return (
            <label className="check" key={p.id}>
              <input type="checkbox" checked={ids.includes(p.id)} onChange={() => toggleId(p.id)} />
              <span>
                {p.name}
                {other && !ids.includes(p.id) && <span className="field__hint"> — сейчас: {other.name}</span>}
              </span>
            </label>
          );
        })}
      </fieldset>
      {error && <p className="form-error span-2" role="alert">{error}</p>}
      <div className="btn-row span-2">
        <button className="btn btn--sm btn--primary" type="button" onClick={submit}>Сохранить</button>
        <button className="btn btn--sm" type="button" onClick={onCancel}>Отмена</button>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: styles.css** — в блок дополнений в конце файла:

```css
/* Лампы: растения под лампой */
.lamp-plants { border: 0; padding: 0; margin: 0; display: grid; gap: 2px; }
.lamp-plants legend { font-size: 13px; color: var(--ink-2); margin-bottom: 4px; }
.fert-form label.check { display: flex; align-items: center; gap: 10px; min-height: 44px; font-size: 15px; color: var(--ink); }
.check input { width: 20px; height: 20px; accent-color: var(--accent); }
```

- [ ] **Step 7: SettingsPage.tsx**:
  - импорты: убрать `LampScheduleEditor`; добавить `import { LampList } from '../components/LampList';`.
  - после `useAsync` страницы добавить отдельную загрузку ламп (чтобы перезагрузка ламп не сбрасывала черновики растений):

```tsx
  const { data: lamps, reload: reloadLamps } = useAsync(() => api.lamps(), []);
```

  - основную загрузку: `useAsync(() => Promise.all([api.plants(), api.settings()]), [])`; `const [plants, app] = data;`.
  - блок «Своя лампа» заменить на:

```tsx
                {selected && (
                  <>
                    <h2 className="section__title" style={{ marginTop: 20 }}>Лампа</h2>
                    <div className="form-group">
                      <Field label="Под какой лампой стоит" hint="Часы этой лампы засчитываются растению">
                        {(id) => (
                          <select
                            className="select"
                            style={{ maxWidth: 220 }}
                            aria-labelledby={id}
                            value={lampOf(selected.id) ?? ''}
                            onChange={(e) => movePlant(selected, e.target.value ? Number(e.target.value) : null)}
                          >
                            <option value="">Без лампы</option>
                            {lamps?.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
                          </select>
                        )}
                      </Field>
                    </div>
                  </>
                )}
```

  - функции внутри компонента (после `removePlant`):

```tsx
  const lampOf = (plantId: number) => lamps?.find((l) => l.plant_ids.includes(plantId))?.id;

  async function movePlant(p: Plant, lampId: number | null) {
    try {
      await api.setPlantLamp(p.id, lampId);
      await reloadLamps();
      toast(lampId ? `${p.name}: ${lamps?.find((l) => l.id === lampId)?.name}` : `${p.name}: без лампы`);
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить');
    }
  }
```

  - `<LightSettings initial={app} schedules={schedules} />` → `<LightSettings initial={app} />`; сразу после него:

```tsx
            <h2 className="section__title" style={{ marginTop: 28 }}>Лампы</h2>
            <LampList lamps={lamps ?? []} plants={plants} onChanged={reloadLamps} />
```

- [ ] **Step 8: Verify** — `cd frontend && npm run build`. Expected: ошибки только в `Dashboard.tsx`, `PlantActions.tsx`, `History.tsx`, `LightHint.tsx` (чинятся в Task 8). Если хочется зелёной сборки на каждом коммите — делать Task 7 и 8 одним коммитом (шаг 5 Task 8).

---

### Task 8: Фронтенд — кнопки, дашборд, подсказка, история

**Files:**
- Modify: `frontend/src/components/PlantActions.tsx`, `frontend/src/pages/Dashboard.tsx`, `frontend/src/components/LightHint.tsx`, `frontend/src/components/History.tsx`

**Interfaces:**
- Consumes: Task 7 (`api.toggleLamp`, `api.toggleLampById`, `api.deleteLampSession`, `LampBrief`, `Lamp`).

- [ ] **Step 1: PlantActions.tsx** — функцию `lamp` заменить на:

```tsx
  const lampBrief = s.light.lamp;

  const lamp = () =>
    run('lamp', async () => {
      const { is_on, session, previous_ended_at, plug_error } = await api.toggleLamp(plantId);
      const base = `${lampBrief?.name ?? 'Лампа'}: ${is_on ? 'включена' : 'выключена'}`;
      const message = plug_error ? `${base}, но розетка не ответила: ${plug_error}` : base;
      return is_on
        ? { message, undo: () => api.deleteLampSession(session.id) }
        : { message, undo: async () => { await api.restoreLampEnd(session.id, previous_ended_at); } };
    });
```

  кнопку лампы:

```tsx
        <button
          className={cls('lamp')}
          type="button"
          aria-pressed={s.lamp.is_on}
          disabled={busy === 'lamp' || !lampBrief}
          title={lampBrief ? undefined : 'Привяжите лампу в настройках'}
          onClick={lamp}
        >
```

- [ ] **Step 2: Dashboard.tsx** — загрузка `useAsync(() => Promise.all([api.summaries(), api.fertilizers(), api.lamps()]), [])`; `const [summaries, fertilizers, lamps] = data ?? [undefined, [], []];`; удалить `sharedOn`/`toggleShared`; импорт типа `Lamp`; добавить:

```tsx
  async function toggle(l: Lamp) {
    try {
      const { is_on, session, previous_ended_at, plug_error } = await api.toggleLampById(l.id);
      reload();
      const base = `${l.name}: ${is_on ? 'включена' : 'выключена'}`;
      toast(plug_error ? `${base}, но розетка не ответила: ${plug_error}` : base, async () => {
        if (is_on) await api.deleteLampSession(session.id);
        else await api.restoreLampEnd(session.id, previous_ended_at);
        reload();
      });
    } catch {
      toast('Не удалось переключить лампу');
    }
  }
```

  тулбар:

```tsx
      {lamps.length > 0 && (
        <div className="toolbar">
          {lamps.map((l) => (
            <button className="chip" type="button" key={l.id} aria-pressed={l.is_on} data-on={l.is_on} onClick={() => toggle(l)}>
              <Icon name="lamp" />
              {l.is_on ? `${l.name} горит` : l.name}
            </button>
          ))}
        </div>
      )}
```

- [ ] **Step 3: LightHint.tsx** — тело функции до `return`:

```tsx
  const auto = light.lamp?.mode === 'auto' ? light.lamp : null;
  const plan = auto?.planned.map((p) => `${fmtTime(p.start)}–${p.end ? fmtTime(p.end) : '…'}`).join(' и ');
  let text: string;
  if (light.deficit_hours <= 0) {
    text = `Света хватает: ${fmtHours(light.total_hours)} ч при норме ${fmtHours(light.target_hours)}`;
    if (plan) text += ` — ${auto!.name} досветит ${plan}`;
  } else if (auto) {
    text = `Не хватает ${fmtHours(light.deficit_hours)} ч даже с досветкой`;
    if (plan) text += ` (${plan})`;
  } else if (light.suggestion_start && light.suggestion_end) {
    text = `Не хватает ${fmtHours(light.deficit_hours)} ч — лампа ${fmtTime(light.suggestion_start)}–${fmtTime(light.suggestion_end)}`;
    if (light.suggestion_until_midnight) text += ', и до полуночи не хватит';
  } else {
    text = `Сегодня не хватило ${fmtHours(light.deficit_hours)} ч света`;
  }
  if (light.natural_hours === null) text += '. Город не задан — солнце не учтено';
```

  докстринг: «…и когда включить лампу (или когда она досветит сама)».

- [ ] **Step 4: History.tsx** — `title: e.shared ? 'Общая лампа' : 'Лампа'` → `title: e.lamp_name ?? 'Лампа'`.

- [ ] **Step 5: Verify** — `cd frontend && npm run build`. Expected: сборка без ошибок.

- [ ] **Step 6: Check in the running app** — `docker compose up -d --build`, открыть `https://polivalochka.cool:1477`: «Настройки → Лампы» — создать «Лампа цитрусы» с Лимоном и Лаймом, режим «Вручную»; на дашборде чип лампы включает/выключает, тост с «Отменить»; у растения в «Настройках» выбор лампы; история показывает имя лампы. Без токена форма лампы показывает подсказку «Сначала вставьте токен…».

- [ ] **Step 7: Commit** (Task 7 + 8 вместе, сборка зелёная)

```bash
git add frontend/src
git commit -m "Лампы во фронте: раздел «Лампы», токен Яндекса, привязка растений, чипы ламп"
```

---

### Task 9: Документация

**Files:** `docs/architecture.md`, `docs/api.md`, `docs/usage.md`, `docs/decisions.md`, `docs/history.md`, `docs/ideas.md`, `CLAUDE.md`

- [ ] **Step 1: CLAUDE.md** — в «Архитектура» заменить пункты про `LampSession.plant_id IS NULL`, «Общая лампа — своя у каждой учётки» и абзац «Свет: …» на:

```markdown
- **Лампы** (`lamps`) — объекты учётки. Растение под лампой — период `plant_lamps` (открытый — не больше одного:
  частичный unique-индекс `uq_plant_lamp_open`, миграция 0004; в моделях его нет — autogenerate не соглашаться удалять,
  как и `uq_lamp_one_open`). Часы растения = сессии ламп его периодов, обрезанные по границам (`clip_session`),
  пересечения объединяются (`lamp_hours_between`). Перенос/удаление лампы (архив, `archived_at`) историю не меняют.
- Режимы лампы: `auto` (досветка до нормы: половина нехватки утром до рассвета, остаток после заката, по максимуму
  среди растений лампы), `schedule` (интервалы `lamp_schedules`), `manual`. Всё превращается в `lamp_sessions`
  (`source`); логика часов работает только с сессиями. Логика — `services/lamps.py`, правила — `summary.py`.
- Розетка — устройство Умного дома Яндекса (`services/yandex.py`, токен учётки зашифрован `secret_box` ключом из
  `JWT_SECRET`). Шаг раз в минуту (`lamps.tick`, lifespan в `main.py`): сессии по расписаниям, досветка, команда
  розетке — только при смене нужного состояния (`lamps.last_state`). Свет по городу — раз в 30 мин (`light.sync_all`).
  В тестах подменять `light.fetch_days`, `yandex.set_on`, `yandex.list_devices` — в сеть не ходить.
```

- [ ] **Step 2: docs/architecture.md** — схема таблиц: `lamps`, `plant_lamps`, `lamp_sessions >─ lamps`, `lamp_schedules >─ lamps`; раздел про розетку («программируемая, не умная») заменить описанием из Step 1 (режимы, шаг раз в минуту, только при смене состояния, ошибки в `lamps.last_error`, `yandex_token_invalid`).

- [ ] **Step 3: docs/api.md** — удалить строки `/lamp-schedules`; добавить таблицу:

```markdown
| GET | `/lamps` | Лампы учётки (без архивных): растения, режим, розетка, горит ли, расписание, досветка на сегодня |
| POST | `/lamps` | `{name, mode: auto/schedule/manual, device_id, device_name, morning_not_before, evening_not_after, plant_ids}`; `auto` без города — 400 |
| GET / PATCH / DELETE | `/lamps/{id}` | PATCH — поля как в POST, `plant_ids` — полная замена; DELETE — в архив (история сохраняется) |
| PUT | `/lamps/{id}/schedule` | `{intervals}` — только в режиме `schedule`; пересечения/конец раньше начала — 400 |
| POST | `/lamps/{id}/toggle` | Кнопка лампы; ответ как у `/lamp-sessions/toggle` + `plug_error` |
| PUT | `/plants/{id}/lamp` | `{lamp_id или null}` — перенести растение |
| GET | `/yandex/devices` | Устройства Умного дома с вкл/выкл; без токена — 400 |
| PUT | `/settings/yandex-token` | `{token или null}` — проверяется у Яндекса; токен в ответах не возвращается, в `GET /settings` — `yandex_status` |
```

  и поправить строки `/lamp-sessions`: `lamp_id` вместо `plant_id`, `toggle {plant_id}` — лампа растения, без лампы 400.

- [ ] **Step 4: docs/usage.md** — раздел про лампу заменить: как создать лампу и привязать растения; режимы; инструкция «Умная розетка»:

```markdown
### Умная розетка (Smart Life → Алиса → Поливалка)
1. Smart Life: добавить розетку (Wi-Fi 2,4 ГГц, зажать кнопку ~5 с), назвать, например, «Лампа цитрусы».
   Подстраховка: в самой розетке расписание «выключить в 23:30 ежедневно», других программ не ставить.
2. Дом с Алисой: «+» → «Устройство умного дома» → Smart Life → привязать аккаунт → обновить список.
3. Токен: oauth.yandex.ru/client/new → «Веб-сервисы», Redirect URI `https://oauth.yandex.ru/verification_code`,
   права «Умный дом»: просмотр и управление → создать. Открыть
   `https://oauth.yandex.ru/authorize?response_type=token&client_id=<ClientID>` → «Разрешить» → скопировать токен.
   Токен живёт около года; истёк — в «Настройки → Свет» будет «недействителен», вставить новый.
4. Поливалка: «Настройки → Свет → Умный дом Яндекса» — вставить токен, «Проверить».
   «Настройки → Лампы» — лампа, розетка из списка, режим «Авто», растения.
Выключили лампу через Алису — Поливалка не включит её назад до следующего переключения по плану.
```

- [ ] **Step 5: docs/decisions.md** — записи:

```markdown
### 23. Лампа — объект, растение под ней — период привязки
Несколько ламп на учётку, растение под одной. Часы растения считаются по лампе, под которой оно стояло тогда
(`plant_lamps`), удаление лампы — архив. Иначе перенос растения или удаление лампы переписывали бы прошлые недели.

### 24. Досветка — по самому требовательному растению, поровну утро/вечер
Одна лампа на несколько растений: горит, пока норму не наберёт самое требовательное (цитрусам лишний свет не вредит).
Утро планируется по прогнозу, вечер пересчитывается до заката по факту — ошибки прогноза компенсируются. Границы
06:00/23:00 — у лампы.

### 25. Розетка — через Умный дом Яндекса, команда только при смене состояния
Сервер на VPS, розетка Tuya облачная: Tuya Cloud требует продлевать пробный пакет, вход по QR Smart Life работает под
чужим client id. Яндекс — один OAuth-токен на год, любая розетка, видимая Алисе. Команда уходит только когда нужное
состояние меняется: ручное выключение через Алису не перебивается каждую минуту. Токен шифруется ключом из JWT_SECRET.
```

- [ ] **Step 6: docs/history.md** — строка: `- 2026-09-25: лампы как объекты, периоды привязки растений, досветка до нормы утро/вечер, умная розетка через Умный дом Яндекса (миграция 0004).`; **docs/ideas.md** — удалить раздел «Умная розетка: приложение само включает лампу».

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md docs
git commit -m "docs: лампы, досветка и умная розетка через Яндекс"
```

---

### Task 10: Финальная проверка и выпуск (только с согласия пользователя)

- [ ] **Step 1:** Все тесты (команда Postgres из шапки) — PASS; `cd frontend && npm run build` — PASS; `docker compose up -d --build` → `docker compose ps` — 4 сервиса `(healthy)`; `docker compose logs backend | tail` — `alembic upgrade` до `0004` без ошибок.
- [ ] **Step 2:** Показать пользователю итог и спросить про выпуск. Выпуск — `bash deploy/release.sh` (бэкап БД перед выкаткой делает сам скрипт). Без подтверждения не запускать.
- [ ] **Step 3:** После выпуска: на проде «Настройки → Лампы» — лампы из миграции («Общая лампа» / «Лампа <растение>») с правильными растениями; история света за прошлые недели не изменилась.
