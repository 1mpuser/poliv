#!/usr/bin/env bash
# Осмотр сервера перед деплоем. Только чтение: .env и секреты не открываются, ничего не меняется.
# Запуск: bash deploy/inspect-server.sh [user@host]
set -u
HOST="${1:-root@213.108.23.47}"
ssh -o ConnectTimeout=10 "$HOST" 'bash -s' <<'REMOTE'
section() { printf "\n== %s\n" "$1"; }
section "Ресурсы";   free -h | head -2; df -h / | tail -1; nproc
section "Контейнеры"; docker ps --format "{{.Names}}\t{{.Image}}\t{{.Ports}}"
section "Слушающие TCP-порты"; ss -ltn | awk 'NR>1{print $4}' | sort -u | tr '\n' ' '; echo
CADDY=$(docker ps --format '{{.Names}}' | grep -i caddy | head -1)
section "Caddy-контейнер: $CADDY"
if [ -n "$CADDY" ]; then
  docker inspect -f 'networks: {{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$CADDY"
  docker inspect -f 'network_mode: {{.HostConfig.NetworkMode}}' "$CADDY"
  docker inspect -f '{{range .Mounts}}{{.Source}} -> {{.Destination}}{{"\n"}}{{end}}' "$CADDY"
  section "Caddyfile в контейнере"; docker exec "$CADDY" cat /etc/caddy/Caddyfile
  section "compose-проект Caddy"; docker inspect -f '{{index .Config.Labels "com.docker.compose.project.working_dir"}} {{index .Config.Labels "com.docker.compose.project.config_files"}}' "$CADDY"
fi
section "/opt"; ls /opt
REMOTE
