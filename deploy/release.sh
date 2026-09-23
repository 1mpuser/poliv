#!/usr/bin/env bash
# Выпуск одной командой: проверки → git push → деплой на сервер → проверка прода.
#
#   bash deploy/release.sh
#
# Требования: всё закоммичено; uv и npm на маке; для API-тестов поднят локальный стек
# (`docker compose up -d`), иначе они пропускаются с предупреждением.
# SKIP_API_TESTS=1 — не запускать API-тесты даже при поднятом стеке.
set -euo pipefail
cd "$(dirname "$0")/.."

HOST="${HOST:-root@213.108.23.47}"
DOMAIN="${DOMAIN:-polivalochka.ru}"
step() { printf "\n==> %s\n" "$1"; }

if [ -n "$(git status --porcelain)" ]; then
  echo "Есть незакоммиченные изменения — закоммитьте их, на сервер уезжает только HEAD." >&2
  git status --short >&2
  exit 1
fi
branch=$(git branch --show-current)
[ "$branch" = master ] || echo "Внимание: ветка $branch, не master."

step "Юнит-тесты бэкенда"
(cd backend && uv run --quiet --python 3.12 --with-requirements requirements-dev.txt pytest -q)

step "API-тесты на Postgres"
if [ "${SKIP_API_TESTS:-0}" = 1 ]; then
  echo "пропущены (SKIP_API_TESTS=1)"
elif [ "$(docker compose ps --format '{{.Service}} {{.Health}}' 2>/dev/null | grep -c '^db healthy')" = 1 ]; then
  docker compose exec -T db sh -c 'createdb -U "$POSTGRES_USER" poliv_test 2>/dev/null || true'
  docker compose run --rm --no-deps -u root -v "$PWD/backend:/app" backend sh -c \
    'export TEST_DATABASE_URL="${DATABASE_URL%/*}/poliv_test";
     pip install -q --root-user-action=ignore pytest httpx >/dev/null 2>&1;
     python -m pytest -q -p no:cacheprovider tests/test_api.py'
else
  echo "ПРОПУЩЕНЫ: локальный стек не поднят (docker compose up -d)."
fi

step "Фронтенд: typecheck + сборка"
(cd frontend && { [ -d node_modules ] || npm ci --silent; } && npm run --silent build >/dev/null && rm -rf dist)
echo ok

step "git push"
git push

step "Деплой $(git rev-parse --short HEAD) → $HOST"
bash deploy/deploy-server.sh

step "Проверка прода"
rev=$(git rev-parse --short HEAD)
deployed=$(ssh "$HOST" "cat /opt/poliv/REVISION 2>/dev/null" || true)
if [ "$deployed" != "$rev" ]; then
  echo "На сервере версия '${deployed:-нет}', ожидалась $rev — деплой не дошёл до конца" >&2
  exit 1
fi
echo "версия $rev"
# --resolve: на маке DNS может перехватывать VPN (sing-box TUN)
ip="${HOST#*@}"
for i in 1 2 3 4 5; do
  if curl -fsS --resolve "$DOMAIN:443:$ip" "https://$DOMAIN/api/health"; then
    echo "  ← https://$DOMAIN"
    exit 0
  fi
  sleep 3
done
echo "Прод не ответил на /api/health" >&2
exit 1
