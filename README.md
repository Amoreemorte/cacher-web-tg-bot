# ИИ-бот учёта семейных расходов

Сервис учёта семейных расходов. Член семьи пишет боту в Telegram «кофе 350», и трата сохраняется: бот сам определяет сумму, категорию и дату.

## Состав системы

1. **Telegram-бот с ИИ-агентом** — CRUD над тратами пользователя на естественном языке. Каждому члену семьи доступны только его траты.
2. **REST API** — бэкенд для веб-панели на чистом `net/http` с CRUD, фильтрами, пагинацией и rate limiting.
3. **Веб-панель для главы семьи** — React + TypeScript + Vite SPA, раздаётся через Nginx, работает с бэкендом через REST API. Авторизация не нужна.

Все компоненты запускаются одной командой:

```bash
docker compose up --build -d
```

---

## Быстрый старт

### 1. Подготовка окружения

Скопируйте шаблон переменных и заполните реальные значения:

```bash
cp .env.example .env
```

Обязательно заполните в `.env`:
- `TELEGRAM_BOT_TOKEN` — токен от @BotFather (без него бот не стартует)
- `LLM_PRIMARY_URL`, `LLM_PRIMARY_KEY`, `LLM_PRIMARY_MODEL` — основной LLM (например, Groq)
- Опционально: `LLM_BACKUP_URL`, `LLM_BACKUP_KEY`, `LLM_BACKUP_MODEL` — резервный LLM

### 2. Запуск

```bash
docker compose up --build -d
```

### 3. Открыть веб-панель

```
http://localhost:8080
```

Порт можно изменить переменной `WEB_PORT` в `.env`.

### 4. Проверить API

```bash
curl http://localhost:8080/healthz
# → {"status":"ok"}
```

### 5. Проверить бота

Напишите боту в Telegram «кофе 350». Траты должны сохраниться в SQLite и появиться в веб-панели.

---

## Переменные окружения (`.env`)

См. `.env.example` — полный шаблон без секретов. Пример минимального `.env`:

```env
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11

LLM_PRIMARY_URL=https://api.groq.com/openai/v1/chat/completions
LLM_PRIMARY_KEY=ВАШ_КЛЮЧ
LLM_PRIMARY_MODEL=llama-3.3-70b-versatile

# Резервный LLM (опционально)
LLM_BACKUP_URL=
LLM_BACKUP_KEY=
LLM_BACKUP_MODEL=

LLM_TIMEOUT_SECONDS=30
LLM_MAX_RETRIES=2
LLM_RETRY_DELAY_SEC=2
LLM_MAX_TOKENS=512

HTTP_ADDR=:8080
RATE_LIMIT_RPS=10
RATE_LIMIT_BURST=20
CORS_ALLOWED_ORIGINS=*

DB_PATH=/data/expenses.db
DEBUG=false

WEB_PORT=8080
```

Файл `.env` не попадает в Git (см. `.gitignore`).

---

## Управление сервисами

| Команда | Смысл |
|---------|-------|
| `docker compose up --build -d` | Собрать и запустить все сервисы |
| `docker compose ps` | Статус сервисов |
| `docker compose logs -f` | Логи всех сервисов |
| `docker compose logs -f api` | Логи только API |
| `docker compose logs -f bot` | Логи только бота |
| `docker compose logs -f web` | Логи только веб-панели |
| `docker compose stop` | Остановить сервисы |
| `docker compose start` | Запустить снова |
| `docker compose down` | Остановить и удалить контейнеры |
| `docker compose down -v` | ⚠️ Удалить контейнеры **и volume с базой данных** (`expenses.db`) |
| `docker compose build api` | Пересобрать отдельный сервис (`api`, `bot` или `web`) |

---

## Данные между перезапусками

SQLite-файл `/data/expenses.db` хранится в именном Docker-volуме `expense-data`, который монтируется в сервисы `api` и `bot`. Данные сохраняются после `docker compose down` и пересборки.

**Полное удаление (включая базу):**

```bash
docker compose down -v   # ВНИМАНИЕ: удаляет volume expense-data вместе с expenses.db
```

---

## Архитектура развёртывания

```
expense-bot_default (сеть)
├── api   — Go REST API (сервер), порт не публикуется, доступ через Nginx
├── bot   — Telegram-бот (тот же образ, другая команда)
└── web   — Nginx со статикой React, порт ${WEB_PORT:-8080}:80

expense-data (volume) → /data/expenses.db (общий для api и bot)
```

- `api` и `bot` используют один и тот же файл SQLite через общий volume.
- `api` имеет healthcheck на `GET /healthz`.
- `web` стартует после того, как `api` станет healthy (`condition: service_healthy`).
- Порт API не публикуется наружу — доступ к нему идёт через Nginx по внутренней Docker-сети.

---

## Проверка работоспособности

- **API:** `curl http://localhost:8080/healthz` → `{"status":"ok"}`
- **Веб-панель:** http://localhost:8080
- **Telegram-бот:** напишите боту «кофе 350» (нужен действительный `TELEGRAM_BOT_TOKEN`)
- **Общая БД:** записи, сделанные ботом, видны в веб-панели и наоборот (один volume `/data`)

---

## Структура проекта

```
.
├── cmd/
│   ├── bot/          # точка входа Telegram-бота
│   └── server/       # точка входа REST API
├── config/           # загрузка конфигурации из переменных окружения
├── internal/
│   ├── bot/          # ядро бота (интерфейсы Sender, UpdateProvider, Handler)
│   ├── domain/       # модели данных (Expense)
│   ├── llm/          # клиент LLM, фейловер primary/backup
│   ├── repository/   # интерфейсы репозиториев и реализация SQLite
│   ├── server/       # REST API (handlers, middleware, server)
│   └── service/      # бизнес-логика, процессор отложенных LLM-задач
├── web/              # React + TypeScript + Vite фронтенд
│   ├── src/
│   ├── Dockerfile
│   ├── nginx.conf
│   └── package.json
├── Dockerfile        # многостейджевый Go-образ (bot + server)
├── compose.yaml      # orchestration: api, bot, web
├── .env.example      # шаблон переменных окружения
├── .env              # локальные значения (не в Git)
├── .dockerignore
├── go.mod / go.sum
└── AGENTS.md         # подробное описание этапов разработки
```

---

## Этапы разработки

- **Этап 1** — базовый эхо-бот (интерфейсы `Sender`, `UpdateProvider`, `Handler`).
- **Этап 2** — слой базы данных, `ExpenseRepository`, SQLite.
- **Этап 3** — интеграция с LLM, определение действия/суммы/категории/даты, фейловер, очередь `pending_llm_tasks`.
- **Этап 4** — REST API на `net/http`, rate limiting, защита от SQL-инъекций, тесты.
- **Этап 5** — веб-панель на React + TypeScript + Vite.
- **Этап 6** — Docker Compose и production-сборка (этот релиз).

---

## Требования

- Docker Desktop / Docker Engine с плагином Compose
- Для работы бота: действительный `TELEGRAM_BOT_TOKEN`
- Для ответов ИИ: доступ к LLM (primary/backup, например Groq / OpenRouter / Ollama)

---

## Ограничения

- Для работы бота нужен реальный `TELEGRAM_BOT_TOKEN`; для ответов ИИ — доступ к LLM (primary/backup).
- Нет авторизации (как и требуется заданием).
- Нет графиков (только таблица трат).
- Docker Desktop daemon должен быть запущен для `docker compose build/up`.