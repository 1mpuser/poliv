# Рабочий цикл: изменил → проверил → выкатил

## Один раз на новом маке

```bash
git clone git@github.com:1mpuser/poliv.git && cd poliv
cp .env.example .env                  # POSTGRES_PASSWORD и JWT_SECRET: openssl rand -hex 32
(cd frontend && npm ci)
docker compose up -d --build
docker compose exec backend /poliv set-owner --email you@example.com   # пароль напечатается
```

Нужны: Docker, Node 20+, Go 1.24+, SSH-ключ к `root@213.108.23.47`.

Локальный домен `polivalochka.cool:1477` дополнительно требует строку в `/etc/hosts` и доверенный
сертификат локального CA Caddy — см. [deploy.md](deploy.md#локальный-стек).

## Каждая доработка

1. **Меняем код.**
   - Фронтенд с hot reload поверх запущенного стека:
     `cd frontend && API_URL=https://polivalochka.cool:1477 npm run dev` → http://localhost:5173
   - Бэкенд: после правки — `docker compose up -d --build backend`.
   - Схема БД — только новой SQL-миграцией (см. ниже).
2. **Проверяем** (`release.sh` сделает это сам, но быстрее ловить раньше):
   ```bash
   cd backend-go && go vet ./... && go test ./internal/summary/ ./internal/passwords/ ./internal/secretbox/ ./internal/yandex/
   cd frontend && npm run build                                                             # типы + сборка
   ```
   Новое правило статусов — сначала тест в `backend-go/internal/summary/summary_test.go`.
   Новый эндпоинт с данными учётки — тест изоляции в `backend-go/internal/api/api_integration_test.go`.
3. **Коммитим** (без AI-трейлеров `Co-Authored-By`).
4. **Выкатываем:**
   ```bash
   bash deploy/release.sh
   ```
   Скрипт: юнит-тесты → API-тесты на Postgres (если локальный стек поднят) → сборка фронтенда →
   `git push` → `deploy/deploy-server.sh` → проверка `https://polivalochka.ru/api/health`.
   Останавливается на первой ошибке; с незакоммиченными изменениями не запускается.

Выкатить без проверок (например, правка только в документации): `bash deploy/deploy-server.sh`.

## Миграции БД

Схема лежит в `backend-go/internal/migrate/sql/*.sql` (простой раннер, версия — таблица `pg_migrations`).
На существующей (ранее Alembic) базе миграции ничего не меняют: базовая `0001_baseline.sql`
применяется только на пустой базе и воссоздаёт схему, идентичную Alembic head (0001–0005),
включая частичные индексы `uq_plant_lamp_open`, `uq_lamp_one_open` и `ON DELETE SET NULL`
у `feeding_logs.fertilizer_type_id`.

Новая миграция — новый файл `NNNN_...sql` и прогон туда-обратно:

```bash
# на пустой базе: подняться с нуля
docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test 2>/dev/null || true'
docker run --rm --network poliv_default -v "$PWD/backend-go:/src" -w /src \
  -e TEST_DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@db:5432/poliv_test" \
  -e JWT_SECRET=test -e TZ=Europe/Moscow golang:1.24-alpine sh -c 'go test ./internal/api/'
```

`poliv_test` пересоздаётся при каждом прогоне; тесты откажутся работать с базой без «test» в имени.

## Все тесты

Юнит (без БД): `cd backend-go && go vet ./... && go test ./...` — API-тесты при этом пропускаются.

API-тесты на реальном Postgres (отдельная база `poliv_test`, не рабочая `poliv`):

```bash
docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test 2>/dev/null || true'
POSTGRES_USER=$(grep '^POSTGRES_USER=' .env | cut -d= -f2)
POSTGRES_PASSWORD=$(grep '^POSTGRES_PASSWORD=' .env | cut -d= -f2)
NETWORK=$(docker inspect poliv-backend-1 --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')
docker run --rm --network "$NETWORK" -v "$PWD/backend-go:/src" -w /src \
  -e TEST_DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@db:5432/poliv_test" \
  -e JWT_SECRET=test-secret -e TZ=Europe/Moscow \
  golang:1.24-alpine sh -c 'go test ./internal/api/'
```
