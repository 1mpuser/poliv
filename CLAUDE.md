# Поливалка — заметки для Claude

Self-hosted трекер ухода за растениями: FastAPI + Postgres + React SPA за Caddy, всё в Docker Compose.
Документация — `docs/` (начинать с `docs/workflow.md`; решения и их причины — `docs/decisions.md`).
Здесь — только то, что нужно при работе с кодом. Меняешь поведение или инфраструктуру — обнови `docs/`
и допиши строку в `docs/history.md`.

## Команды

```bash
docker compose up -d --build              # весь стек; миграции применяются при старте backend
docker compose ps                         # у всех 4 сервисов должно быть (healthy)
docker compose logs -f backend              # сервисы: db, backend, poliv-web, caddy

# юнит-тесты (Python 3.12 через uv; системный python3 — 3.9, на нём код не импортируется)
cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q

# все тесты, включая tests/test_api.py на реальном Postgres (база poliv_test пересоздаётся;
# без TEST_DATABASE_URL API-тесты пропускаются)
docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test' 2>/dev/null
docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
  'export TEST_DATABASE_URL="${DATABASE_URL%/*}/poliv_test"; pip install -q pytest httpx && python -m pytest -q'

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
- Сезон и порог «скоро» (`notify_days_ahead`) — в `user_settings`, остальные настройки ухода — поля `Plant`.
- `FeedingLog.fertilizer_type_id` — `ON DELETE SET NULL`: удаление удобрения не трогает историю.
- **Мультиучётки, данные изолированы.** `plants`, `fertilizer_types`, `lamp_sessions` имеют `user_id`,
  журналы принадлежат учётке через растение, настройки — `user_settings` (PK = `user_id`).
  Любой доступ по id — через `crud.owned_*`: чужая запись отвечает 404, как несуществующая.
  Новый эндпоинт без `CurrentUser` и `owned_*` — дыра; на изоляцию есть тесты в `tests/test_api.py`.
- Вход: OAuth2 password form, `username` = почта. JWT: `sub` = id, `ver` = `users.token_version`;
  смена пароля и блокировка увеличивают версию — старые токены сразу 401.
- Учётки выдаёт админ (`/api/admin/*`, для не-админа 404; себя заблокировать/удалить нельзя) или CLI
  `python -m app.cli` (`set-owner`, `create-user`, `reset-password`, `make-admin`). Пароли — scrypt (`passwords.py`).
  Миграция 0002 отдала старые данные заглушке `owner@localhost.invalid` (id=1) — её «оживляет» `set-owner`.
- PATCH-эндпоинты используют `crud.apply_update` (`exclude_unset`): явный `null` — значимое значение
  (например, `ended_at: null` снова зажигает лампу — так работает «Отменить»).
- Город — в `user_settings`; норма света — поля `Plant`.

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

## Прод

- https://polivalochka.ru — VPS `root@213.108.23.47`, код в `/opt/poliv`, рядом трекер perfotracker.ru и VPN.
- TCP 443 держит Caddy трекера (`/opt/tracker/caddy/Caddyfile.prod`, блок между `# poliv:begin/end`).
  Свой Caddy на сервере не запускается: `docker-compose.server.yml` подключает `poliv-web` к сети `tracker_default`.
- Поэтому имена сервисов уникальны: `poliv-web` (не `frontend`), бэкенд для nginx — алиас `poliv-api`.
  Сервис с именем `frontend`/`backend` в сети трекера перехватит трафик трекера.
- Пароли учёток генерируются на сервере и печатаются один раз — Claude их не видит
  (классификатор блокирует чтение секретов); CLI-команды с выводом пароля отдавать пользователю через `!`.
- Порт 80 на сервере не трогать (acme.sh для VPN), UDP 443 не публиковать (hysteria). Сертификат — TLS-ALPN.
- Бэкап БД: root-cron 03:25 → `/var/backups/poliv/db`, 14 дней (`deploy/backup.sh`).
- Выпуск: `bash deploy/release.sh` (тесты → push → деплой → проверка прода); только деплой — `bash deploy/deploy-server.sh` (уезжает HEAD, `.env` на сервере создаётся один раз).

## Правила

- Отвечать на русском. В коммитах — без трейлеров `Co-Authored-By` и прочей AI-атрибуции.
- Схему БД менять только новой миграцией Alembic, не правкой `0001_initial.py`.
