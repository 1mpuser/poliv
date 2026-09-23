#!/usr/bin/env bash
# Деплой на VPS, где TCP 443 уже держит Caddy трекера (perfotracker.ru).
#
#  1. Код текущего коммита (git archive HEAD) → /opt/poliv
#  2. /opt/poliv/.env создаётся один раз, секреты генерируются на сервере и сюда не выводятся
#  3. db + backend + poliv-web поднимаются без своего Caddy (docker-compose.server.yml),
#     poliv-web подключается к сети трекера
#  4. В Caddyfile трекера один раз дописывается блок домена (между маркерами poliv:begin/end),
#     конфиг проверяется `caddy validate`; при ошибке файл восстанавливается из бэкапа.
#     Затем `caddy reload` — без перезапуска контейнера, трекер не прерывается.
#
# Запуск: bash deploy/deploy-server.sh            (повторный запуск = обновление кода)
# Откат блока в Caddy: удалить строки между маркерами в Caddyfile.prod и
#   docker exec tracker-caddy-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
set -euo pipefail

HOST="${HOST:-root@213.108.23.47}"
DOMAIN="${DOMAIN:-polivalochka.ru}"
REMOTE_DIR="${REMOTE_DIR:-/opt/poliv}"
EDGE_NETWORK="${EDGE_NETWORK:-tracker_default}"
EDGE_CADDY="${EDGE_CADDY:-tracker-caddy-1}"
EDGE_CADDYFILE="${EDGE_CADDYFILE:-/opt/tracker/caddy/Caddyfile.prod}"

cd "$(dirname "$0")/.."
if [ -n "$(git status --porcelain)" ]; then
  echo "Есть незакоммиченные изменения — на сервер уедет только HEAD ($(git rev-parse --short HEAD))." >&2
fi

echo "==> Код $(git rev-parse --short HEAD) → $HOST:$REMOTE_DIR"
ssh "$HOST" "mkdir -p '$REMOTE_DIR'"
git archive --format=tar HEAD | ssh "$HOST" "tar -x -C '$REMOTE_DIR'"

ssh "$HOST" \
  "DOMAIN='$DOMAIN' REMOTE_DIR='$REMOTE_DIR' EDGE_NETWORK='$EDGE_NETWORK' EDGE_CADDY='$EDGE_CADDY' EDGE_CADDYFILE='$EDGE_CADDYFILE' bash -s" <<'REMOTE'
set -euo pipefail
cd "$REMOTE_DIR"

if [ ! -f .env ]; then
  echo "==> Создаю .env (секреты генерируются здесь, на сервере)"
  umask 077
  cat > .env <<ENV
DOMAIN=$DOMAIN
TZ=Europe/Moscow
POSTGRES_DB=poliv
POSTGRES_USER=poliv
POSTGRES_PASSWORD=$(openssl rand -hex 24)
APP_USERNAME=admin
APP_PASSWORD=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)
JWT_SECRET=$(openssl rand -hex 32)
JWT_EXPIRE_DAYS=30
EDGE_NETWORK=$EDGE_NETWORK
ENV
fi

echo "==> Сборка и запуск (без своего Caddy)"
COMPOSE="docker compose -f docker-compose.yml -f docker-compose.server.yml"
$COMPOSE up -d --build --remove-orphans

echo "==> Жду healthy"
for i in $(seq 1 60); do
  n=$($COMPOSE ps --format '{{.Health}}' | grep -c '^healthy$' || true)
  [ "$n" -ge 3 ] && break
  sleep 3
done
$COMPOSE ps --format '{{.Service}}\t{{.Status}}'
[ "$n" -ge 3 ] || { echo "Сервисы не стали healthy" >&2; exit 1; }

# Проверка изнутри Caddy трекера: имя poliv-web резолвится и отвечает, а «frontend» по-прежнему трекера
docker exec "$EDGE_CADDY" wget -qO- http://poliv-web/api/health; echo " ← poliv через сеть трекера"

if grep -q '# poliv:begin' "$EDGE_CADDYFILE"; then
  echo "==> Блок $DOMAIN в Caddyfile трекера уже есть — не трогаю"
else
  echo "==> Добавляю $DOMAIN в Caddyfile трекера"
  BACKUP="$EDGE_CADDYFILE.bak-before-poliv-$(date +%Y%m%d%H%M%S)"
  cp -p "$EDGE_CADDYFILE" "$BACKUP"
  # >> сохраняет inode: файл примонтирован в контейнер bind-mount'ом
  cat >> "$EDGE_CADDYFILE" <<CADDY

# poliv:begin — Поливалка (/opt/poliv), добавлено deploy-server.sh
$DOMAIN {
	import tls_alpn_only
	encode zstd gzip

	header {
		Strict-Transport-Security "max-age=31536000"
		X-Content-Type-Options "nosniff"
		Referrer-Policy "strict-origin-when-cross-origin"
		X-Frame-Options "DENY"
		-Server
	}

	reverse_proxy poliv-web:80
}

www.$DOMAIN {
	import tls_alpn_only
	redir https://$DOMAIN{uri} permanent
}
# poliv:end
CADDY
  if ! docker exec "$EDGE_CADDY" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/tmp/poliv-validate.log 2>&1; then
    cat /tmp/poliv-validate.log >&2
    cat "$BACKUP" > "$EDGE_CADDYFILE"   # восстановление с тем же inode
    echo "Конфиг не прошёл проверку — Caddyfile восстановлен из $BACKUP" >&2
    exit 1
  fi
  echo "    бэкап: $BACKUP"
fi

echo "==> caddy reload"
docker exec "$EDGE_CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile

TRACKER_DOMAIN=$(docker exec "$EDGE_CADDY" printenv DOMAIN || true)
if [ -n "$TRACKER_DOMAIN" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' --resolve "$TRACKER_DOMAIN:443:127.0.0.1" "https://$TRACKER_DOMAIN/" || true)
  echo "==> Трекер $TRACKER_DOMAIN после reload: HTTP $code"
fi
echo "==> Готово. Пароль входа: ssh $(whoami)@<сервер> grep APP_PASSWORD $REMOTE_DIR/.env"
REMOTE
