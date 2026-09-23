# API

Живая документация со схемами: **https://polivalochka.ru/api/docs** (Swagger; «Authorize» — почта и пароль).

Всё под `/api`. Без токена открыты только `/health` и `/auth/token`. Ответ на чужую запись — 404.
PATCH меняет только переданные поля; явный `null` — тоже значение (`ended_at: null` снова зажигает лампу).

## Вход и аккаунт

| Метод | Путь | |
|---|---|---|
| POST | `/auth/token` | form: `username` (почта), `password` → `{access_token}` |
| GET | `/auth/me` | `{id, email, is_admin}` |
| POST | `/auth/password` | `{current_password, new_password}` → новый токен, остальные разлогиниваются |

## Админка (только `is_admin`, иначе 404)

| Метод | Путь | |
|---|---|---|
| GET | `/admin/users` | Список учёток |
| POST | `/admin/users` | `{email, password}` → 201; занятая почта 409, короткий пароль 400 |
| POST | `/admin/users/{id}/password` | `{password}` → 204, токены учётки отзываются |
| POST | `/admin/users/{id}/block` · `/unblock` | 204; себя — 400 |
| DELETE | `/admin/users/{id}` | 204, со всеми данными; себя — 400 |

## Растения

| Метод | Путь | |
|---|---|---|
| GET / POST | `/plants` | Список / создать |
| GET | `/plants/summary` | **Сводки всех растений одним запросом** (дашборд) |
| GET / PATCH / DELETE | `/plants/{id}` | |
| GET | `/plants/{id}/summary` | Сводка: `water`, `feed` (следующее удобрение и срок), `lamp` (часы сегодня), `repot` |
| GET | `/plants/{id}/history` | Лента событий; `?types=water&types=feed`, `date_from`, `date_to` (YYYY-MM-DD) |
| GET | `/plants/{id}/stats/weekly` | `?weeks=8` (1–52): поливы, подкормки, часы лампы по неделям |

## Журналы

Одинаковый CRUD: `GET /X?plant_id=&limit=`, `POST /X`, `GET|PATCH|DELETE /X/{id}`.

| X | Тело POST | |
|---|---|---|
| `waterings` | `{plant_id, watered_at?, note?}` | время по умолчанию — сейчас |
| `feedings` | `{plant_id, fertilizer_type_id, method: root\|foliar, fed_at?, note?}` | |
| `repottings` | `{plant_id, repotted_at?, pot_size_before?, pot_size_after?, note?}` | обновляет `pot_size_l` растения |

## Свет

| Метод | Путь | |
|---|---|---|
| GET | `/light/geocode?q=Мытищи` | Поиск города (Open-Meteo) → `[{name, region, country, latitude, longitude}]` |
| GET | `/light/today` | Свет сегодня в городе учётки или `null` |
| GET | `/lamp-schedules` | Все расписания ламп учётки |
| PUT | `/lamp-schedules` | `{plant_id или null, intervals: [{start_time: "07:00", end_time: "10:00"}]}` — полная замена расписания одной лампы; пустой список — убрать. Конец раньше начала или пересечения — 400 |

Город задаётся через `PATCH /settings` (`location_name`, `latitude`, `longitude`). В сводке растения
блок `light`: норма, солнечные часы, световой день, восход/закат, часы лампы по плану, итог, нехватка,
окно-подсказка, расписания своей и общей лампы.

## Лампа

| Метод | Путь | |
|---|---|---|
| POST | `/lamp-sessions/toggle` | `{plant_id}` или `{plant_id: null}` (общая лампа) → `{is_on, session, previous_ended_at}`: гасит горящую (в т.ч. по расписанию), иначе включает |
| GET | `/lamp-sessions` | `?plant_id=`, `?shared=true`, `?open=true` |
| POST | `/lamp-sessions` | Ручная сессия; вторая горящая на то же растение → 409 |
| GET / PATCH / DELETE | `/lamp-sessions/{id}` | |

## Удобрения и настройки

| Метод | Путь | |
|---|---|---|
| GET / POST | `/fertilizers` | Имя уникально в пределах учётки (409) |
| GET / PATCH / DELETE | `/fertilizers/{id}` | Удаление не стирает историю подкормок |
| GET / PATCH | `/settings` | `{current_season: active\|dormant, notify_days_ahead: 0–14, location_name, latitude, longitude}` |
