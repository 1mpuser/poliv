# Архитектура

## Сервисы

```
                ┌──────────── локально ────────────┐     ┌────────────── на сервере ──────────────┐
браузер ─HTTPS─▶│ caddy (свой)                     │     │ tracker-caddy-1 (Caddy трекера, :443)  │
                │   └─▶ poliv-web:80               │     │   ├─ perfotracker.ru → трекер          │
                └──────────────────────────────────┘     │   └─ polivalochka.ru → poliv-web:80    │
                                                         └────────────────────────────────────────┘
poliv-web (nginx):  /api/* ─▶ poliv-api:8000 (backend)     остальное ─▶ собранный SPA (index.html fallback)
backend (FastAPI):  миграции Alembic при старте ─▶ db (PostgreSQL 16, volume pgdata)
```

| Сервис | Образ | Роль |
|---|---|---|
| `db` | `postgres:16-alpine` | Данные в volume `pgdata` |
| `backend` | `backend/Dockerfile` (python 3.12-slim) | FastAPI + SQLAlchemy 2 + Alembic; в сети — алиас `poliv-api` |
| `poliv-web` | `frontend/Dockerfile` (node → nginx) | Статика SPA и прокси `/api` на backend — единственная точка входа |
| `caddy` | `caddy:2-alpine` | Только локально: HTTPS перед `poliv-web`. На сервере отключён (`docker-compose.server.yml`) |

Health-check есть у всех четырёх. Порты наружу публикует только `caddy` (локально).

## Модель данных

```
users ─┬─< plants ─┬─< watering_logs
       │           ├─< feeding_logs >─ fertilizer_types (ON DELETE SET NULL)
       │           ├─< repotting_logs
       │           └─< lamp_sessions (plant_id NULL = общая лампа учётки)
       ├─< fertilizer_types          (имя уникально в пределах учётки)
       ├─< lamp_sessions
       └── user_settings (PK = user_id: сезон, notify_days_ahead)
```

- Все внешние ключи на `users` и `plants` — `ON DELETE CASCADE`: удаление учётки или растения
  уносит всю его историю.
- `plants` хранит и описание (name, species, location, pot_size_l, notes), и настройки ухода:
  `water_interval_days`, `fertilizing_enabled`, `lamp_hours_per_day`, `repot_check_interval_months`.
- Частичный уникальный индекс `uq_lamp_one_open (user_id, COALESCE(plant_id, 0)) WHERE ended_at IS NULL`:
  одна горящая лампа на растение и одна общая на учётку. Повторное включение → 409.
- Все времена — `timestamptz`.
- Миграции: `0001` — схема и стартовые данные, `0002` — мультиучётки (старые данные → заглушка владельца id=1).

## Правила статусов

Считаются **только на бэкенде**, фронтенд показывает готовые поля `GET /api/plants/summary`.
Чистые функции без БД — `backend/app/services/summary.py` (тесты — `tests/test_summary.py`);
сборка из БД — `services/plants.py`. Дни календарные, в поясе `TZ` (Europe/Moscow).

Статус: `ok` (зелёный) · `soon` (жёлтый, «скоро») · `late` (красный, «пора») · `off` (серый, выключено).

| Что | Как считается |
|---|---|
| Полив | `due_in = water_interval_days − дней с полива`; `late` при ≤0, `soon` при ≤ `notify_days_ahead`. Не поливали — `late` |
| Подкормка, какой тип | Следующий по кругу (по id) после типа последней подкормки — «не тот, что в прошлый раз». Нет истории — первый. Последний тип удалён — следующий по id |
| Подкормка, когда | Дата последней + интервал **следующего** типа для текущего сезона. Не подкармливали — сегодня. Выключена у растения, нет удобрений или у типа нет интервала для сезона покоя — `off` |
| Лампа | Часы за сегодня: сессии растения ∪ общей лампы учётки, пересечения объединяются, горящая считается до «сейчас». ≥90% нормы — `ok`, ≥50% — `soon`, меньше — `late` |
| Пересадка | Следующая проверка = последняя пересадка (или дата добавления) + N месяцев; `soon` за 14 дней |
| Статистика | Недели с понедельника, последняя — текущая неполная: поливы, подкормки, часы лампы |

## Авторизация и учётки

- Вход: `POST /api/auth/token` (OAuth2 password form, `username` = почта) → JWT HS256
  (`sub` = id учётки, `ver` = `users.token_version`, срок `JWT_EXPIRE_DAYS`).
- `current_user` на каждом запросе читает учётку из БД: заблокирована или `ver` не совпал → 401.
  Смена пароля и блокировка увеличивают `token_version` — все выданные токены умирают сразу.
- Пароли — scrypt (stdlib), `backend/app/passwords.py`. Неверный пароль → пауза 1 с.
- Изоляция: доступ по id только через `crud.owned_plant / owned_fertilizer / owned_log / owned_lamp`;
  чужая запись отвечает 404, как несуществующая. Списки фильтруются по `user_id`.
- Админка `/api/admin/*` — только `is_admin`, остальным 404. Подробнее — [accounts.md](accounts.md).

## Фронтенд

React 19 + Vite + TypeScript + react-router 7, без UI-библиотек и стейт-менеджеров.

- Вёрстка повторяет макет `design/`: `src/styles.css` = `design/styles.css` + блок дополнений в конце.
  Классы и data-атрибуты макета (`.stat[data-status]`, `.action[aria-pressed]`, `.chart__bars --max`,
  `.bar__seg --n`) — контракт, не переименовывать. Цвета — только токены `:root`.
- Тема: светлая/тёмная по системе, переключатель в настройках (localStorage).
- Экраны (`src/pages`): дашборд, растение (плитки, действия, 2 графика за 8 недель, история с фильтрами),
  настройки (растение, сезон, напоминания, удобрения, тема, аккаунт), новое растение, админка, вход.
- Данные: `useAsync` + локальный state; все запросы — `src/api.ts` (401 → экран входа).
- Быстрые действия (полив, подкормка, лампа, пересадка) показывают тост «Отменить», который удаляет
  созданную запись (для выключения лампы — снова открывает сессию).
- Числа, даты, склонения — `src/format.ts`. Текст интерфейса — только русский.
