# Рабочий цикл: изменил → проверил → выкатил

## Один раз на новом маке

```bash
git clone git@github.com:1mpuser/poliv.git && cd poliv
cp .env.example .env                  # POSTGRES_PASSWORD и JWT_SECRET: openssl rand -hex 32
(cd frontend && npm ci)
docker compose up -d --build
docker compose exec backend python -m app.cli set-owner --email you@example.com   # пароль напечатается
```

Нужны: Docker, Node 20+, [uv](https://docs.astral.sh/uv/) (он сам поставит Python 3.12),
SSH-ключ к `root@213.108.23.47`.

Локальный домен `polivalochka.cool:1477` дополнительно требует строку в `/etc/hosts` и доверенный
сертификат локального CA Caddy — см. [deploy.md](deploy.md#локальный-стек).

## Каждая доработка

1. **Меняем код.**
   - Фронтенд с hot reload поверх запущенного стека:
     `cd frontend && API_URL=https://polivalochka.cool:1477 npm run dev` → http://localhost:5173
   - Бэкенд: после правки — `docker compose up -d --build backend`.
   - Схема БД — только новой миграцией (см. ниже).
2. **Проверяем** (`release.sh` сделает это сам, но быстрее ловить раньше):
   ```bash
   cd backend && uv run --python 3.12 --with-requirements requirements-dev.txt pytest -q   # юнит
   cd frontend && npm run build                                                          # типы + сборка
   ```
   Новое правило статусов — сначала тест в `backend/tests/test_summary.py`.
   Новый эндпоинт с данными учётки — тест изоляции в `backend/tests/test_api.py`.
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

```bash
# после правки backend/app/models.py
docker compose exec backend alembic revision --autogenerate -m "что меняется"
docker compose cp backend:/app/alembic/versions/. backend/alembic/versions/
```

Сгенерированный файл **читать руками**:

- autogenerate предлагает удалить частичный индекс `uq_lamp_one_open` (его нет в моделях) — убрать из миграции;
- переименования он видит как drop + add — переписать на `op.alter_column(..., new_column_name=...)`;
- у каждой миграции должен быть рабочий `downgrade`.

Проверка туда-обратно на тестовой базе:

```bash
docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
  'export DATABASE_URL="${DATABASE_URL%/*}/poliv_test"; alembic upgrade head && alembic downgrade -1 && alembic upgrade head'
```

На сервере миграции применяются сами при старте backend. Перед выкаткой миграции, меняющей данные,
снять ручной бэкап (см. [deploy.md](deploy.md#бэкапы)).

## Все тесты

```bash
docker compose exec db sh -c 'createdb -U "$POSTGRES_USER" poliv_test' 2>/dev/null
docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
  'export TEST_DATABASE_URL="${DATABASE_URL%/*}/poliv_test"; pip install -q pytest httpx && python -m pytest -q'
```

`poliv_test` пересоздаётся при каждом прогоне; тесты откажутся работать с базой без «test» в имени.
