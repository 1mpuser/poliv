#!/usr/bin/env bash
# Ежедневный дамп БД поливалки (cron на сервере). Хранится 14 дней.
set -euo pipefail
cd "$(dirname "$0")/.."
DEST=/var/backups/poliv/db
mkdir -p "$DEST"
docker compose -f docker-compose.yml -f docker-compose.server.yml exec -T db \
  sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$DEST/poliv-$(date +%F).dump"
find "$DEST" -name 'poliv-*.dump' -mtime +14 -delete
