# Поливалка

Self-hosted трекер ухода за комнатными растениями: полив, подкормки с чередованием удобрений,
досветка лампой, пересадки. Изначально — лимон Мейера и лайм, новые растения добавляются через интерфейс.

Прод: https://polivalochka.ru · Подробная документация — [`docs/`](docs/README.md) · Выкатить изменения: `bash deploy/release.sh`

## Стек

| Сервис | Что внутри |
|---|---|
| `db` | PostgreSQL 16, данные в volume `pgdata` |
| `backend` | FastAPI + SQLAlchemy 2 + Alembic (миграции применяются при старте) |
| `poliv-web` | React 19 + Vite + TypeScript, собирается в статику и отдаётся nginx; он же проксирует `/api` |
| `caddy` | Реверс-прокси с HTTPS → `poliv-web` |

## Запуск

```bash
cp .env.example .env
# Задайте POSTGRES_PASSWORD и JWT_SECRET:  openssl rand -hex 32
docker compose up -d --build
docker compose ps          # у всех четырёх сервисов должен быть статус (healthy)

# первый админ: пароль сгенерируется и напечатается один раз
docker compose exec backend python -m app.cli set-owner --email you@example.com
```

- Приложение: https://localhost — вход по почте и паролю
- Swagger: https://localhost/api/docs (кнопка **Authorize** принимает почту и пароль)

## Учётки

Самостоятельной регистрации нет — учётки выдаёт админ во вкладке **«Админка»**: почта (это логин) +
пароль, который генерируется в форме и показывается один раз с кнопкой «Скопировать». Там же —
новый пароль, блокировка и удаление учётки со всеми данными. У каждой учётки свои растения,
удобрения и настройки; новая начинается пустой, с двумя стартовыми удобрениями.

Смена пароля или блокировка сразу разлогинивает учётку на всех устройствах. Свой пароль меняется
в «Настройках» → «Аккаунт».

Команды на сервере (пароль генерируется, если не задан `--password`):

```bash
docker compose exec backend python -m app.cli set-owner --email you@example.com   # владелец-админ
docker compose exec backend python -m app.cli create-user --email friend@example.com [--admin]
docker compose exec backend python -m app.cli reset-password --email you@example.com  # если забыли
docker compose exec backend python -m app.cli make-admin --email friend@example.com
```

Сертификат выпускает локальный CA Caddy (`TLS=internal`), поэтому браузер покажет предупреждение,
пока корневой сертификат не добавлен в доверенные (см. ниже).

## Локальный домен с портом (пример: polivalochka.cool:1477)

```bash
# .env
DOMAIN=polivalochka.cool:1477
HTTPS_PORT=1477
TLS=internal

docker compose up -d
echo "127.0.0.1 polivalochka.cool" | sudo tee -a /etc/hosts

# доверить локальный CA Caddy (macOS; сертификат переживает пересоздание контейнера,
# пока жив volume caddy_data)
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./caddy-root.crt
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain caddy-root.crt
```

Открывайте https://polivalochka.cool:1477. Caddy сам продлевает сертификат сайта (он живёт ~12 ч),
корневой действует 10 лет. Firefox использует своё хранилище — там сертификат импортируется в настройках.

## Смена домена (прод)

1. Направьте A/AAAA-запись домена на сервер, откройте порты 80 и 443.
2. В `.env`: `DOMAIN=plants.example.com`, `HTTPS_PORT=443`, `TLS=you@example.com`
   (email для Let's Encrypt вместо `internal`).
3. `docker compose up -d` — Caddy сам получит и будет продлевать сертификат.

Код менять не нужно. Чтобы работать по чистому HTTP без сертификата (например, за другим прокси),
задайте `DOMAIN=:80`.

## Сервер, где 443 уже занят другим приложением

Если на сервере уже работает реверс-прокси (здесь — Caddy трекера), свой Caddy не запускается:

```bash
docker compose -f docker-compose.yml -f docker-compose.server.yml up -d --build
```

`poliv-web` подключается к docker-сети внешнего прокси (`EDGE_NETWORK`, по умолчанию `tracker_default`),
а в его конфиг добавляется `reverse_proxy poliv-web:80` для нового домена. Для текущего сервера всё это
делает `bash deploy/deploy-server.sh` (осмотр сервера без изменений — `bash deploy/inspect-server.sh`).

## Бэкап и восстановление

Логический дамп (рекомендуется — переносим между версиями Postgres):

```bash
mkdir -p backups
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backups/poliv-$(date +%F).dump

# восстановление в пустую базу
docker compose exec -T db sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < backups/poliv-2026-09-23.dump
```

Снимок всего volume (при остановленной базе):

```bash
docker compose stop db
docker run --rm -v poliv_pgdata:/data -v "$PWD/backups":/backup alpine \
  tar czf /backup/pgdata-$(date +%F).tar.gz -C /data .
docker compose start db
# восстановление: распакуйте архив обратно в volume poliv_pgdata тем же контейнером alpine
```

Для ежедневного бэкапа достаточно строки `pg_dump` в cron на хосте.

## Как считаются статусы

Вся логика — на бэкенде (`backend/app/services/summary.py`, покрыта тестами), фронтенд только показывает
`GET /api/plants/{id}/summary` (или `GET /api/plants/summary` — все растения одним запросом).
Дни календарные, в часовом поясе `TZ` (Europe/Moscow).

- **Полив**: дни с последнего полива против нормы растения «раз в N дней».
  Красный — срок наступил, жёлтый — осталось не больше `notify_days_ahead` дней.
- **Подкормка**: следующий тип — следующий по кругу после последнего использованного (по id), то есть не тот,
  что был в прошлый раз. Срок = дата последней подкормки + интервал **следующего** типа для текущего сезона.
  Если у удобрения не задан интервал для сезона покоя — в покой им не подкармливают (статус «выкл.»).
  Тумблер «Подкормка» у растения отключает напоминания.
- **Лампа**: сумма сессий за сегодня, пересечения своей и общей лампы не суммируются дважды.
  Зелёный — не меньше 90% нормы часов, жёлтый — не меньше 50%, красный — меньше.
  Сессия с `plant_id = null` — общая лампа, её часы засчитываются всем растениям.
- **Пересадка**: следующая проверка = последняя пересадка (или дата добавления) + N месяцев; жёлтый — за 14 дней.

## Структура

```
.
├── docker-compose.yml, Caddyfile, .env.example
├── design/                  исходный статичный макет (HTML/CSS)
├── backend/
│   ├── alembic/versions/    миграции (0001 — схема + стартовые данные)
│   ├── app/
│   │   ├── main.py          FastAPI, /api, Swagger
│   │   ├── auth.py          JWT, вход по почте, текущая учётка
│   │   ├── cli.py           set-owner, create-user, reset-password, make-admin
│   │   ├── models.py        SQLAlchemy-модели
│   │   ├── schemas.py       Pydantic-схемы
│   │   ├── routers/         plants, fertilizers, logs, lamp, settings, admin
│   │   └── services/        summary.py — правила, plants.py — сборка из БД
│   └── tests/
└── frontend/
    └── src/
        ├── api.ts, types.ts, format.ts
        ├── components/      карточка, плитки, действия, график, история, формы
        └── pages/           Dashboard, PlantPage, SettingsPage, NewPlant, AdminPage, Login
```

## Разработка

```bash
# юнит-тесты (правила, пароли)
cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest

# все тесты, включая API на реальном Postgres (база poliv_test пересоздаётся)
docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test' 2>/dev/null
docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
  'export TEST_DATABASE_URL="${DATABASE_URL%/*}/poliv_test"; pip install -q pytest httpx && python -m pytest -q'

# фронтенд с hot reload поверх запущенного стека (прокси /api → https://localhost)
cd frontend && npm install && npm run dev

# новая миграция после изменения models.py
docker compose exec backend alembic revision --autogenerate -m "описание"
```
