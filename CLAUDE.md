# Поливалка — заметки для Claude

Self-hosted трекер ухода за растениями: FastAPI + Postgres + React SPA за Caddy, всё в Docker Compose.
Пользовательская документация — в `README.md`, здесь только то, что нужно при работе с кодом.

## Команды

```bash
docker compose up -d --build              # весь стек; миграции применяются при старте backend
docker compose ps                         # у всех 4 сервисов должно быть (healthy)
docker compose logs -f backend

# тесты правил (Python 3.12 через uv; системный python3 — 3.9, на нём код не импортируется)
cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q

# фронтенд: typecheck + сборка (это и есть «линт» — ESLint в проекте нет)
cd frontend && npm run build

# dev-сервер фронта с прокси /api на запущенный стек
cd frontend && API_URL=https://polivalochka.cool:1477 npm run dev   # по умолчанию https://localhost

# новая миграция после правки models.py — проверить сгенерированный файл руками
docker compose exec backend alembic revision --autogenerate -m "..."
```

Порт backend наружу не опубликован: API трогать через Caddy (`https://$DOMAIN/api/...`)
или `docker compose exec -T backend python -` со скриптом на urllib.

## Архитектура

- **Правила статусов — только на бэкенде.** `backend/app/services/summary.py` — чистые функции без БД
  (всё время приходит аргументами, покрыто `tests/test_summary.py`). `services/plants.py` достаёт данные
  из БД и собирает `PlantSummary`. Фронтенд статусы не вычисляет, только показывает поля сводки
  (`ok | soon | late | off`). Новое правило — сначала тест в `test_summary.py`.
- Дни — календарные, в `settings.zone` (env `TZ`, Europe/Moscow). В БД всё `timestamptz`.
- `LampSession.plant_id IS NULL` — общая лампа, её часы засчитываются всем растениям; пересечения
  объединяются (`lamp_hours_between`). Одна открытая сессия на растение — частичный unique-индекс
  `uq_lamp_one_open` в миграции 0001 (в моделях его нет, autogenerate может предложить его удалить — не соглашаться).
- `AppSettings` — одна строка `id=1` (CHECK). Сезон и порог «скоро» (`notify_days_ahead`) глобальные,
  остальные настройки ухода — поля `Plant`.
- `FeedingLog.fertilizer_type_id` — `ON DELETE SET NULL`: удаление удобрения не трогает историю.
- Все роуты под `/api`, всё кроме `/api/health` и `/api/auth/token` закрыто JWT (`auth.require_user`
  на уровне роутера в `main.py`). Логин — OAuth2 password form, единственный пользователь из `.env`.
- PATCH-эндпоинты используют `crud.apply_update` (`exclude_unset`): явный `null` — значимое значение
  (например, `ended_at: null` снова зажигает лампу — так работает «Отменить»).

## Фронтенд

- Вёрстка повторяет макет из `design/` (статичный HTML — эталон). `frontend/src/styles.css` = `design/styles.css`
  + блок дополнений в конце. Классы и data-атрибуты (`.stat[data-status]`, `.action[aria-pressed]`,
  `.chart__bars --max`, `.bar__seg --n`, …) — контракт макета, не переименовывать.
- Цвета — только токены из `:root` (светлая/тёмная тема через `prefers-color-scheme` и `data-theme`).
- Весь текст интерфейса на русском; числа и даты — через `src/format.ts` (`plural`, `fmtDate`, `fmtHours`).
- Запросы — только через `src/api.ts`. Быстрые действия показывают тост с «Отменить» (`useToast`).
- Без UI-библиотек и стейт-менеджеров: `useAsync` + локальный state.

## Инфраструктура

- `.env` не коммитится (есть `.env.example`). `DOMAIN` может содержать порт; он должен совпадать с `HTTPS_PORT`.
- `TLS=internal` — сертификат от локального CA Caddy, `TLS=<email>` — Let's Encrypt.
  Локально сейчас `DOMAIN=polivalochka.cool:1477` (нужна строка в `/etc/hosts` и доверенный `caddy-root.crt`).
- Volumes: `pgdata` (данные), `caddy_data` (CA и сертификаты). `docker compose down -v` стирает базу и локальный CA —
  после этого корневой сертификат придётся доверить заново.

## Правила

- Отвечать на русском. В коммитах — без трейлеров `Co-Authored-By` и прочей AI-атрибуции.
- Схему БД менять только новой миграцией Alembic, не правкой `0001_initial.py`.
