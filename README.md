# VideoHub — дипломный проект

Простой видеохостинг типа YouTube: пользовательская часть (главная, просмотр,
каналы, подписки, личный кабинет) + админ-панель с накруткой статистики.

* **Frontend:** React 18 + TypeScript + Vite + TailwindCSS + Zustand + TanStack Query.
* **Backend:** Go 1.22 (chi, pgx, JWT, bcrypt).
* **БД:** PostgreSQL 16. Схема и сидинг применяются автоматически при старте.
* **Контейнеризация:** docker compose — единая команда для всего стека.

## Структура

```
.
├── src/                  # frontend (Vite)
├── backend/              # Go backend (cmd/server, internal/*, migrations/)
├── nginx.conf            # SPA + прокси /api /uploads на backend
├── Dockerfile            # frontend образ
├── docker-compose.yml    # postgres + backend + frontend + pgadmin
└── pgadmin/servers.json  # авторегистрация сервера в pgadmin
```

## Быстрый запуск (Docker)

Требуется **Docker Desktop** (или Docker Engine + Compose).

```bash
docker compose up -d --build
```

Поднимется четыре сервиса:

| Сервис    | URL                       | Описание                                            |
|-----------|---------------------------|-----------------------------------------------------|
| frontend  | http://localhost:8081     | SPA, прокси `/api` и `/uploads` на backend          |
| backend   | http://localhost:8080     | Go API (миграции + сидинг применяются автоматически)|
| postgres  | localhost:**5433**        | Внутри сети `postgres:5432`                         |
| pgadmin   | http://localhost:5050     | Вход: `admin@videohub.com` / `admin`                |

Остановить с сохранением всех данных:

```bash
docker compose down       # видео, аватарки, БД и всё прочее переживёт перезапуск
```

Полный сброс (стереть БД и pgadmin; **uploads сохранятся** — они bind-mount'ом
лежат в `./backend/uploads/MP4` и `./backend/uploads/PNG`):

```bash
docker compose down -v
```

### Где хранятся данные

| Что               | Где                                  | Переживёт `down -v` |
|-------------------|--------------------------------------|---------------------|
| Загруженные видео | `./backend/uploads/MP4/` (на хосте)  | Да (bind-mount)     |
| Превью / аватарки | `./backend/uploads/PNG/` (на хосте)  | Да (bind-mount)     |
| База PostgreSQL   | named volume `postgres_data`         | Нет                 |
| Настройки pgadmin | named volume `pgadmin_data`          | Нет                 |

Файлы пользователей не пропадают при штатных перезапусках. Если делаешь
`down -v`, файлы остаются на диске, но ссылки на них в БД исчезают вместе со
схемой — после нового `up` сидинг создаст пустую таблицу `videos`, а лежащие
на диске mp4/png не подтянутся обратно. Чтобы избежать этого, не используй
флаг `-v` при штатной остановке.

## Транскрипция и субтитры

После загрузки видео backend автоматически запускает Python-worker
(`backend/worker/transcribe.py`), который:

1. **Распознаёт речь** через [faster-whisper](https://github.com/SYSTRAN/faster-whisper)
   с авто-детектом языка (модель `WHISPER_MODEL=small` по умолчанию, CPU/int8).
2. **Сохраняет** оригинальную расшифровку и таймкоды (`segments`) в БД.
3. **Переводит** сегменты на два других языка из набора **ru / kk / en**
   через локальный [Argos Translate](https://github.com/argosopentech/argos-translate).
   Если пакет перевода недоступен — статус для этого языка становится `FAILED`,
   остальные языки и оригинал остаются `COMPLETED`.
4. **Пишет WebVTT-файлы** в `backend/uploads/SUBTITLES/<videoId>_<lang>.vtt`.
5. **Генерирует таймкод-главы** автоматически из сегментов
   (раз в ~45 секунд, до 12 глав).

В UI просмотра видео появляются:
- блок **«Расшифровка»** с переключателем языка ru / kk / en и кликабельными сегментами;
- блок **«Таймкоды»** под описанием — кликабельные главы в стиле YouTube;
- субтитры внутри плеера через нативные `<track>` (выбор в нативном меню плеера / `c` для CC, если включить controls).

API:

| Метод | Путь                                          | Что делает                                    |
|-------|-----------------------------------------------|-----------------------------------------------|
| GET   | `/api/videos/{id}/transcript?lang=ru\|kk\|en` | Текст + сегменты + статус для языка           |
| GET   | `/api/videos/{id}/subtitles`                  | Какие языки готовы и ссылки на VTT            |
| POST  | `/api/videos/{id}/transcribe`                 | Перезапустить распознавание (auth)            |
| POST  | `/api/videos/{id}/translate`                  | Перезапустить переводы (auth)                 |

### Установка зависимостей

В Docker всё уже включено — на этапе сборки `Dockerfile` ставит `ffmpeg`,
`libgomp1`, Python, faster-whisper, argostranslate, psycopg.

Без Docker (локальный запуск):

```bash
# Системные пакеты
sudo apt-get install -y ffmpeg python3 python3-pip
# Или на macOS:  brew install ffmpeg
# Или на Windows:  winget install Gyan.FFmpeg

# Python-зависимости worker'а
pip install -r backend/worker/requirements.txt
```

Затем backend подхватит worker через переменные:
- `PYTHON_BIN=python3`
- `WHISPER_WORKER=$(pwd)/backend/worker/transcribe.py`
- `WHISPER_MODEL=tiny`/`small`/`medium` — меньшая модель = быстрее, ниже качество.

### Проверка субтитров

1. Залогинься (любой demo-аккаунт), `/me/upload` — загрузи короткий ролик с речью.
2. Открой страницу видео. Снизу появится блок «Расшифровка» со статусом
   `Распознаём речь…`. Через 1–5 минут (зависит от длины + модели) появятся
   сегменты на оригинальном языке + переводы.
3. Переключи язык в блоке «Расшифровка» — `Русский / Қазақша / English` —
   сегменты должны отобразиться на нужном языке (или показать
   `Перевод создаётся…` для языков, на которые Argos ещё качает пакет).
4. Открой VTT файл напрямую: `http://localhost:8080/uploads/SUBTITLES/<videoId>_ru.vtt`.
5. Клик по таймкоду в блоке «Таймкоды» под описанием перематывает плеер.

Если worker отвалился (например, в окружении без Python) — backend пишет
`transcript_status=FAILED`, UI показывает «Не удалось создать расшифровку»
и предлагает перезапустить (для админов — кнопка перезапуска).

## Демо-аккаунты

При первом запуске seed создаёт 10 пользователей. Для входа на http://localhost:8081/login:

| Логин          | Пароль     | Роль          |
|----------------|------------|---------------|
| `admin`        | `admin1`   | администратор |
| `tech_guru`    | `password` | пользователь  |
| `music_master` | `password` | пользователь  |
| `gamer_pro`    | `password` | пользователь  |
| `edu_channel`  | `password` | пользователь  |
| `sport_zone`   | `password` | пользователь  |
| `ivan`         | `password` | пользователь  |
| `cooking`      | `password` | пользователь  |
| `travel`       | `password` | пользователь  |
| `art_studio`   | `password` | пользователь  |

Видео в seed нет — лента изначально пустая. Залогинься, открой **Загрузить** и
залей mp4/webm + превью. После этого видео появится в ленте, на канале и
в `/admin/videos`.

## Запуск без Docker (для разработки)

**Postgres** должен быть доступен по `DATABASE_URL` из `.env`.

```bash
# Backend
cd backend
cp .env.example .env       # при необходимости поправь креды
go mod download
go run ./cmd/server

# Frontend (в другом терминале)
npm install
npm run dev                # http://localhost:5173
```

В дев-режиме фронт ходит на тот же origin — задай `VITE_API_BASE_URL=http://localhost:8080`,
если backend на другом порту.

## Основные возможности

### Пользователь
- Главная страница с рекомендациями (формула `views + likes*5 - dislikes*3 + freshnessBonus`).
- Просмотр видео: кастомный плеер на react-player с собственными контролами
  (play/pause, прогресс-бар с drag, громкость, скорость 0.25–3×, fullscreen).
- Лайки / дизлайки, комментарии, подписки на каналы.
- Личный кабинет: редактирование профиля и канала, загрузка видео (mp4/webm) и
  превью (png/jpg/webp), статистика своего канала на графиках.
- Поиск по названию/описанию/тегам.

### Админ
- Просмотр пользователей, каналов, видео, комментариев, общей статистики.
- Блокировка/разблокировка пользователей.
- Удаление видео и комментариев.
- **Накрутка статистики**:
  - на канале — подписчики, общие просмотры/лайки/дизлайки;
  - на отдельном видео — просмотры, лайки, дизлайки.

### API
Полный список эндпоинтов и схему — см. [backend/README.md](backend/README.md).

## Окружение

Все переменные backend описаны в [backend/.env.example](backend/.env.example).
Ключевые:

| Переменная           | Что делает                                                 |
|----------------------|------------------------------------------------------------|
| `DATABASE_URL`       | Подключение к PostgreSQL                                   |
| `JWT_SECRET`         | Секрет подписи JWT (в проде — длинная случайная строка)   |
| `AUTH_EXPOSE_CODE`   | В демо: возвращать код подтверждения email в ответе API   |
| `ADMIN_USERNAME/PASSWORD` | Документация (хэш закладывается в seed)               |
| `RESEED_ON_START`    | `true` — пересоздать схему и накатить seed при старте      |

## Безопасность для деплоя

Перед публикацией:
1. Смени `JWT_SECRET` на случайные ~64 байта.
2. Поставь `AUTH_EXPOSE_CODE=false` и подключи реальную отправку email.
3. Удали учётку `admin/admin1` или поменяй пароль в seed.
4. Сузь `CORS_ORIGINS` до своего домена.

## Лицензия

Учебный проект.
