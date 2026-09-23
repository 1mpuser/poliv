# Документация Поливалки

Всё, что нужно, чтобы дорабатывать проект и сразу выкатывать его на сервер.

| Документ | О чём |
|---|---|
| [workflow.md](workflow.md) | **Начать отсюда.** Цикл «изменил → проверил → выкатил» одной командой |
| [architecture.md](architecture.md) | Сервисы, модель данных, правила статусов, авторизация, фронтенд |
| [deploy.md](deploy.md) | Сервер, как устроен прод, первый деплой, обновление, откат, бэкапы, диагностика |
| [accounts.md](accounts.md) | Учётки: админка, CLI, восстановление доступа |
| [api.md](api.md) | Все эндпоинты API (живой Swagger — `/api/docs`) |
| [decisions.md](decisions.md) | Принятые решения и почему именно так — читать перед тем, как что-то «упрощать» |
| [ideas.md](ideas.md) | Идеи на будущее, ещё не сделанные |
| [history.md](history.md) | Что и когда сделано |

Коротко:

- Прод: **https://polivalochka.ru** — VPS `213.108.23.47` (Ubuntu 26.04, 1 vCPU / 1,9 ГБ),
  рядом трекер perfotracker.ru и VPN.
- Репозиторий: `github.com:1mpuser/poliv`, ветка `master`.
- Локально: https://polivalochka.cool:1477 (или https://localhost).
- Выкатить: `bash deploy/release.sh`.
- Макет интерфейса (эталон вёрстки): `design/`.
- Заметки для Claude: `CLAUDE.md` в корне.
