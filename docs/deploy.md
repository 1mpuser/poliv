# Сервер и деплой

## Как устроен прод

| | |
|---|---|
| Адрес | https://polivalochka.ru (`www` → редирект 301) |
| Сервер | `root@213.108.23.47`, Ubuntu 26.04, 1 vCPU / 1,9 ГБ + swap 2 ГБ, Docker 29 |
| Код | `/opt/poliv` — распакованный `git archive HEAD`, **не** git-клон; задеплоенный коммит — `/opt/poliv/REVISION` |
| Конфиг | `/opt/poliv/.env` (chmod 600): `DOMAIN`, `TZ`, `POSTGRES_*`, `JWT_*`, `EDGE_NETWORK`. Секреты созданы на сервере |
| Запуск | `docker compose -f docker-compose.yml -f docker-compose.server.yml …` (без своего Caddy) |
| Контейнеры | `poliv-db-1`, `poliv-backend-1`, `poliv-poliv-web-1` |
| DNS | reg.ru: A `@` и `www` → 213.108.23.47; NS `ns1/ns2.reg.ru` |
| Сертификат | Let's Encrypt, выпускает и продлевает Caddy трекера через TLS-ALPN на 443 |
| Бэкап БД | root-cron `25 3 * * *` → `/var/backups/poliv/db/poliv-ГГГГ-ММ-ДД.dump` (14 дней) + `predeploy-*.dump` перед каждой выкаткой (последние 10) |
| Внешние сервисы | Open-Meteo (`api.open-meteo.com`, `geocoding-api.open-meteo.com`) — исходящий HTTPS для света |

### Соседи на сервере — что нельзя трогать

- **Трекер perfotracker.ru** (`/opt/tracker`) — его Caddy (`tracker-caddy-1`) держит TCP 443
  и проксирует оба домена. В его `/opt/tracker/caddy/Caddyfile.prod` дописан наш блок между
  маркерами `# poliv:begin` / `# poliv:end`.
- **VPN**: порт **80** держит acme.sh (сертификат VPN в standalone-режиме), **UDP 443** редиректится
  в hysteria (8443), 10443 — VLESS xhttp. Ни один из них не публиковать и не занимать.
- `poliv-web` подключён к сети `tracker_default`. Поэтому сервисы поливалки **не должны**
  называться `frontend`/`backend`/`caddy`/`postgres` — Docker DNS начнёт отдавать трекеру наш
  контейнер. Отсюда `poliv-web` и алиас `poliv-api`.

## Обновление

```bash
bash deploy/release.sh        # проверки + push + деплой + проверка прода (см. workflow.md)
bash deploy/deploy-server.sh  # только деплой текущего HEAD
```

`deploy-server.sh` идемпотентен:

1. `git archive HEAD` → `/opt/poliv` (незакоммиченное не уезжает — скрипт предупредит);
2. создаёт `.env`, если его нет (секреты генерирует на сервере и не выводит);
3. снимает бэкап БД `predeploy-*.dump`, затем `up -d --build`, ждёт healthy у всех трёх сервисов;
4. ставит cron бэкапа, если его нет;
5. проверяет, что Caddy трекера достаёт `poliv-web` по сети;
6. дописывает блок в Caddyfile трекера, **только если его нет**; перед этим бэкап в
   `/var/backups/poliv/`, после — `caddy validate`, при ошибке файл восстанавливается;
7. `caddy reload` (без перезапуска контейнера) и проверка, что трекер отвечает;
8. пишет коммит в `/opt/poliv/REVISION` — `release.sh` сверяет его с HEAD.

В удалённой части скрипта (она идёт в `bash -s` через stdin) любые `docker compose exec` — только с `</dev/null`,
иначе команда прочитает остаток скрипта как свой ввод и деплой тихо оборвётся.

Сборка на сервере занимает 1–2 минуты (1 vCPU). Даунтайм — несколько секунд при пересоздании контейнеров.

## Первый деплой на чистый сервер

1. DNS: A-записи домена → IP сервера (проверка: `https://dns.google/resolve?name=<домен>`).
2. На сервере — Docker с compose-плагином; внешний прокси (здесь Caddy трекера) с сетью `EDGE_NETWORK`.
3. `bash deploy/inspect-server.sh` — осмотр без изменений: ресурсы, контейнеры, порты, Caddyfile.
4. `HOST=root@<ip> DOMAIN=<домен> bash deploy/deploy-server.sh`
   (переменные: `REMOTE_DIR`, `EDGE_NETWORK`, `EDGE_CADDY`, `EDGE_CADDYFILE` — см. шапку скрипта).
5. Владелец-админ — [accounts.md](accounts.md#первый-вход).

Если на сервере нет чужого прокси, проще обычный режим со своим Caddy:
`DOMAIN=<домен>`, `HTTPS_PORT=443`, `TLS=<email для Let's Encrypt>` в `.env` и `docker compose up -d --build`.

## Откат

```bash
# код: задеплоить предыдущий коммит
git checkout <коммит> && bash deploy/deploy-server.sh && git checkout master

# если откатываемая версия меняла схему — сначала откатить миграцию на сервере
ssh root@213.108.23.47 'cd /opt/poliv && docker compose -f docker-compose.yml -f docker-compose.server.yml \
  exec -T backend alembic downgrade -1'
```

Убрать поливалку из Caddy трекера: удалить строки между `# poliv:begin` и `# poliv:end` в
`/opt/tracker/caddy/Caddyfile.prod` **с сохранением файла на месте** (не `sed -i`, который меняет inode
bind-mount'а — править через `cat бэкап > файл` или редактор, пишущий в тот же файл), затем
`docker exec tracker-caddy-1 caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile`.

## Бэкапы

```bash
# ручной дамп перед рискованной выкаткой
ssh root@213.108.23.47 'cd /opt/poliv && docker compose -f docker-compose.yml -f docker-compose.server.yml \
  exec -T db sh -c "pg_dump -U \$POSTGRES_USER -d \$POSTGRES_DB -Fc" > /var/backups/poliv/db/manual-$(date +%F-%H%M).dump'

# скачать на мак
scp root@213.108.23.47:/var/backups/poliv/db/poliv-2026-09-24.dump .

# восстановить (стирает текущие данные!)
ssh root@213.108.23.47 'cd /opt/poliv && docker compose -f docker-compose.yml -f docker-compose.server.yml \
  exec -T db sh -c "pg_restore -U \$POSTGRES_USER -d \$POSTGRES_DB --clean --if-exists"' < poliv-2026-09-24.dump
```

Существующие бэкапы: `poliv-before-0002.dump` — состояние до мультиучёток.
Бэкап исходного Caddyfile трекера — `/var/backups/poliv/Caddyfile.prod.bak-before-poliv-*`.

## Диагностика

```bash
H='ssh root@213.108.23.47'
C='cd /opt/poliv && docker compose -f docker-compose.yml -f docker-compose.server.yml'
$H "$C ps"                                   # статусы и health
$H "$C logs --tail 100 backend"              # ошибки API, миграции
$H 'docker logs --since 10m tracker-caddy-1 2>&1 | grep polivalochka'   # сертификат, прокси
curl -s --resolve polivalochka.ru:443:213.108.23.47 https://polivalochka.ru/api/health
```

| Симптом | Причина / что делать |
|---|---|
| 502 от polivalochka.ru | `poliv-web` не в сети трекера или не запущен: `$H "$C ps"`, `docker network inspect tracker_default` |
| Трекер иногда отдаёт страницы поливалки | Сервис с именем `frontend`/`backend` попал в `tracker_default` — переименовать |
| Нет сертификата | DNS не указывает на сервер. Caddy повторяет сам; ускорить — `caddy reload --force` |
| С мака домен «не существует», а с сервера резолвится | sing-box TUN перехватывает DNS — проверять через DoH или `--resolve` |
| `git pull` в деплое трекера падает на `Caddyfile.prod` | Наш блок — локальная правка в его репо: `git stash && git pull && git stash pop` |
| Все запросы 401 после выкатки | Сменился `JWT_SECRET` в `.env` — это нормально, войти заново |
| «Данные о свете ещё не получены» | Сервер не достучался до Open-Meteo: `$H "$C logs backend \| grep poliv.light"`; повтор — каждые 30 минут |
| Лампа по расписанию не учитывается | Сессии на день создаёт фоновая задача backend — проверить, что он жив, и `GET /api/lamp-schedules` |

## Локальный стек

```bash
docker compose up -d --build                      # db, backend, poliv-web, caddy
```

`.env` локально: `DOMAIN=polivalochka.cool:1477`, `HTTPS_PORT=1477`, `TLS=internal` (или `DOMAIN=localhost`,
`HTTPS_PORT=443`). Для домена-заглушки один раз:

```bash
echo "127.0.0.1 polivalochka.cool" | sudo tee -a /etc/hosts
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./caddy-root.crt
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain caddy-root.crt
```

`docker compose down -v` стирает базу **и** локальный CA — сертификат придётся доверить заново.
