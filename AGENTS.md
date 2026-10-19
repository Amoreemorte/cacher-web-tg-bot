# Тестовое задание: ИИ-бот учёта расходов

## Цель работы
Сервис учёта семейных расходов. Член семьи пишет боту в Telegram «кофе 350», и трата сохраняется: бот сам определяет сумму, категорию и дату.

## Архитектура
Слоистая архитектура с обязательным использованием интерфейсов для мок-тестирования или замены частей приложения на иные при необходимости.

## Состав системы
1. **Telegram-бот с ИИ-агентом** — CRUD над тратами пользователя на естественном языке. Каждому члену семьи доступны только его траты.
2. **Веб-панель для главы семьи** — показывает траты всех членов семьи и позволяет их редактировать. Работает с бэкендом через REST API. Авторизация не нужна.

## Модель данных: Трата
| Поле | Смысл |
|---|---|
| `id` | идентификатор траты |
| `user_id` | Telegram ID владельца |
| `amount` | сумма |
| `category` | категория: еда, транспорт, жильё, развлечения, здоровье, покупки, другое |
| `spent_at` | дата траты |
| `comment` | необязательный комментарий |
| `created_at`, `updated_at` | время создания и изменения |

## Telegram-бот
Бот реализует CRUD над тратами пользователя на естественном языке, без кнопок и команд:
- добавить: «кофе 350», «такси 1200 вчера»;
- посмотреть: «сколько потратил на еду в этом месяце», «покажи траты за неделю»;
- изменить: «последнее было не 350, а 450»;
- удалить: «удали такси».
Чужие траты в боте недоступны.

## Веб-панель
- список трат с фильтром по члену семьи, периоду и категории;
- создание, редактирование, удаление трат всех членов семьи;
- взаимодействие с бэкендом через REST API.

## Требования
- Бэкенд на Go, фронтенд на любом стеке.
- Приложение запускается со всеми зависимостями через один `docker compose`.
- Доступ к LLM настраивается через конфиг.

## Этап 1 (выполнено)
Базовый эхо-бот:
- При получении текстового сообщения отвечает им же и выводит текст в консоль.
- При получении сообщения без текста отвечает: «Не получил задание, отправьте ещё раз».
- Конфигурация через переменные окружения (`TELEGRAM_BOT_TOKEN`, `DEBUG`).
- Ядро бота зависит только от интерфейсов `Sender`, `UpdateProvider`, `Handler` — легко мокать и заменять.
- Зависимость: `github.com/go-telegram-bot-api/telegram-bot-api/v5`.
- Компиляция проверена: `go build ./...` проходит успешно.

## Этап 2 (выполнено)
- Слой базы данных с интерфейсом `ExpenseRepository`.
- Хранилище: SQLite (однофайловая БД, простая сборка через `docker compose`).

## Этап 3 (выполнено)
- Интеграция с LLM: определение действия, суммы, категории, даты по тексту пользователя.
- JSON-структура ответа от LLM: `action`, `user_id`, `amount`, `category`, `period`, `comment`, `expense_id` (`period` — семантическая метка: `"" | today | yesterday | week | month`, дату не возвращает)
- Обработка ошибок LLM (4xx/5xx): повтор на primary, фейловер на backup, очередь задач в `pending_llm_tasks`.
- Подключение к LLM настраивается через конфиг (`LLM_PRIMARY_URL`, `LLM_BACKUP_URL`, `LLM_TIMEOUT_SECONDS`, и т.д.).
- **Формат взаимодействия с LLM**: OpenAI-совместимый Chat Completions API (`/v1/chat/completions`) — работает с Groq, OpenRouter, Together AI, Ollama и любыми другими совместимыми провайдерами. Клиент отправляет system-prompt с инструкцией возвращать строгий JSON и user-сообщение, парсит `choices[0].message.content`, извлекает JSON (включая markdown-обёртку ```json ... ```).
- **Ограничение `max_tokens`** (`LLM_MAX_TOKENS`, по умолчанию 512): Groq считает лимит OTPM по ожидаемым output-токенам запроса — без ограничения возможен 429 даже при коротком ответе.
- **Дата и время траты устанавливаются только на сервере** в момент записи в БД (`time.Now()`). LLM не возвращает даты — только семантическую метку периода (`today`, `yesterday`, `week`, `month`). Это полностью исключает галлюцинации дат (например, 23.05.2024 вместо текущего дня).
- **Контекст для update/delete**: перед запросом к LLM сервис добавляет список последних 10 трат пользователя (id, сумма, категория, дата, комментарий) — без него LLM не может вернуть `expense_id`.
- **Контекст для update/delete**: перед запросом к LLM сервис добавляет список последних 10 трат пользователя (id, сумма, категория, дата, комментарий) — без него LLM не может вернуть `expense_id`. Текущее время также передаётся в сообщении к LLM.

## Этап 4 (выполнено)
- REST API сервер для веб-панели на чистом `net/http` (без внешних веб-фреймворков)
- Эндпоинты: `GET /api/v1/expenses` (список с фильтрами user_id, категория, период, пагинация), `POST /api/v1/expenses` (создание), `PUT /api/v1/expenses/{id}` (обновление), `DELETE /api/v1/expenses/{id}` (удаление)
- Глобальный rate limiting (токен-бакет через `golang.org/x/time/rate`, лимит настраивается через `RATE_LIMIT_RPS` и `RATE_LIMIT_BURST`)
- Защита от SQL-инъекций: все запросы к БД используют параметризованные prepared statements (через `database/sql` + `modernc.org/sqlite`), строковая конкатенация в SQL не допускается
- Тесты на каждый эндпоинт с использованием `httptest` и временной SQLite БД

## Бесплатные нейросети для LLM-агента

### Основная (Primary)
| Сервис | Бесплатный лимит | Как получить ключ |
|---|---|---|
| **Groq** | до 14 400 токенов в минуту (модели Llama-3, Mixtral) | groq.com → API Keys → Create API Key |
| **OpenRouter** | бесплатные модели (llama-3.1-8b, gemma-2 и др.) | openrouter.ai → Keys → Create Key |
| **Ollama (локально)** | безлимитно (своё железо) | ollama.com → скачать → `ollama run llama3.1` |

### Резервный (Backup)
| Сервис | Бесплатный лимит | Как получить ключ |
|---|---|---|
| **Together AI** | $5 кредитов при регистрации | together.ai → API → API Keys |
| **OpenRouter** (другая модель) | бесплатные модели | тот же ключ, что и выше |
| **Ollama (локально)** | безлимитно | второй экземпляр Ollama или другая модель |

### Переменные окружения для .env
```
TELEGRAM_BOT_TOKEN=ВАШ_ТОКЕН
LLM_PRIMARY_URL=https://api.groq.com/openai/v1/chat/completions
LLM_PRIMARY_KEY=ВАШ_ГРОК_КЛЮЧ
LLM_PRIMARY_MODEL=llama-3.3-70b-versatile
LLM_BACKUP_URL=https://openrouter.ai/api/v1/chat/completions
LLM_BACKUP_KEY=ВАШ_ОПЕН_РОУТЕР_КЛЮЧ
LLM_BACKUP_MODEL=meta-llama/llama-3.3-70b-instruct:free
LLM_TIMEOUT_SECONDS=30
LLM_MAX_RETRIES=2
LLM_RETRY_DELAY_SEC=2
LLM_MAX_TOKENS=512
```

После добавления ключей в `.env` бот будет полностью функционален.

## Обработка ошибок LLM (4xx/5xx)

Стратегия устойчивости: **повтор на primary → фейловер на backup → очередь задач в `pending_llm_tasks`**.

1. При ошибке 4xx/5xx от основного LLM — до `LLM_MAX_RETRIES` повторов к тому же провайдеру с задержкой `LLM_RETRY_DELAY_SEC`.
2. Если не помогло — попытка через резервный LLM (`LLM_BACKUP_URL`).
3. Если оба недоступны — запрос сохраняется в таблицу `pending_llm_tasks` (`id`, `user_id`, `raw_text`, `attempts`, `next_attempt_at`, `created_at`), пользователю отвечается: «LLM не отвечает, попробуйте позже».
4. **При старте бота** и **в фоновой горутине** (каждые 30 сек) процессор `PendingTaskProcessor` проверяет `pending_llm_tasks`, пытается отправить задачи повторно. При успехе — выполняет действие (добавление/обновление/удаление в БД) и удаляет задачу. При неудаче — увеличивает `attempts`, сдвигает `next_attempt_at`. После `maxAttempts` (по умолчанию 5) задача удаляется из очереди.

## Как запустить бота

### Команда для проверки (PowerShell, Windows)
```powershell
$env:TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
$env:LLM_PRIMARY_URL="https://api.groq.com/openai/v1/chat/completions"
$env:LLM_PRIMARY_KEY="ВАШ_GROQ_API_KEY"
$env:LLM_PRIMARY_MODEL="llama-3.3-70b-versatile"
# Опционально: резервный LLM
# $env:LLM_BACKUP_URL="https://openrouter.ai/api/v1/chat/completions"
# $env:LLM_BACKUP_KEY="ВАШ_OPENROUTER_API_KEY"
# $env:LLM_BACKUP_MODEL="meta-llama/llama-3.3-70b-instruct:free"
go run ./cmd/bot
```

### Как получить API-ключи
| Сервис | Для чего | Где получить |
|---|---|---|
| **Groq** (рекомендуется как primary) | Основной LLM, быстро и бесплатно | groq.com → консоль → API Keys → Create API Key |
| **OpenRouter** | Резервный LLM, бесплатные модели | openrouter.ai → Keys → Create Key |
| **Ollama** | Локальная модель без ключа | ollama.com → скачать → `ollama run llama3.1` |

### Что проверить после запуска
1. Напишите боту «кофе 350» — бот ответит «Добавлено: еда 350.00 на ...» и запись сохранится в SQLite.
2. Напишите «удали кофе» — бот ответит «Трата удалена.» и запись удалится из БД.
3. Напишите «покажи траты на еду» — бот ответит списком трат с суммой и категорией.
4. Если LLM недоступна — бот ответит «LLM не отвечает, попробуйте позже», а запрос сохранится в `pending_llm_tasks` для повторной отправки.

## Правила разработки
- После каждого теста тестовая база данных чистится (удаляются все записи из `expenses` и `pending_llm_tasks` или пересоздаётся файл БД).

## Этап 5 (выполнено)

Веб-панель для главы семьи: одностраничное React-приложение с таблицей трат, фильтрами, формой создания/редактирования и подтверждением удаления.

### Стек
- **React 18** + **TypeScript 5.6** + **Vite 5.4**
- Чистый CSS (без UI-библиотек: Tailwind, MUI, Ant Design и т.д.)
- Нативный `fetch` (без axios, SWR, React Query и т.д.)
- Без графиков и сложной архитектуры (только CRUD + фильтры)

### Структура `web/`
```
web/
├── src/
│   ├── api/
│   │   ├── client.ts      # REST-клиент: listExpenses, createExpense, replaceExpense, deleteExpense
│   │   └── types.ts       # Интерфейсы (Expense, ExpensePayload, ExpenseFilters), утилиты форматирования
│   ├── components/
│   │   ├── ConfirmDialog.tsx  # Модальное подтверждение удаления (focus trap, Escape)
│   │   ├── ExpenseForm.tsx    # Форма создания/редактирования (валидация, переиспользование)
│   │   ├── ExpenseTable.tsx   # Адаптивная таблица (desktop → grid, mobile → карточки)
│   │   ├── Filters.tsx        # Фильтры по ID, категории, периоду (today/week/month/all)
│   │   └── StatusViews.tsx    # Loading/Error/Empty/Flash состояния
│   ├── hooks/
│   │   └── useExpenses.ts     # Хук: загрузка, фильтры, пагинация, удаление, flash-сообщения
│   ├── App.tsx            # Главный компонент: оркестрация формы, таблицы, модалки
│   ├── App.css            # Глобальные стили, кнопки, пагинация
│   └── main.tsx           # Точка входа: createRoot + StrictMode
├── index.html             # HTML-шаблон
├── package.json           # Зависимости и скрипты
├── tsconfig.json          # Конфигурация TypeScript (strict, noUnusedLocals)
├── vite.config.ts         # Vite + React + dev-прокси на Go-бэкенд
├── .env.example           # Пример переменных окружения
├── .gitignore             # node_modules, dist, .env.local
└── README.md              # Инструкция запуска
```

### Функциональность
- **Просмотр**: таблица с датой, категорией, комментарием, ID, суммой, кнопками «Изменить»/«Удалить»
- **Фильтры**: `user_id` (числовой), `category` (dropdown), `period` (сегодня/неделя/месяц/всё время)
- **Пагинация**: кнопки «Назад»/«Вперёд», индикатор текущей страницы
- **Создание**: кнопка «+ Добавить трату» → форма с полями `user_id`, `amount`, `category`, `spent_at`, `comment`
- **Редактирование**: кнопка «Изменить» в таблице → та же форма, предзаполненная данными
- **Удаление**: кнопка «Удалить» → модальное подтверждение → API `DELETE`
- **Состояния**:
  - Загрузка: спиннер «Загружаем траты…»
  - Ошибка: баннер с сообщением и кнопкой «Повторить»
  - Пусто: заглушка «Трат пока нет» + кнопка «Добавить трату»
  - Flash: уведомление «Трата удалена» / «Не удалось удалить трату»
- **Адаптивность**: таблица на desktop → grid-карточки на mobile (< 720px)
- **Без авторизации**: как и требуется

### Конфигурация (`.env`)
| Переменная | По умолчанию | Смысл |
|---|---|---|
| `VITE_API_BASE_URL` | (пусто) | базовый URL API; пусто = same-origin (через Vite proxy) |
| `VITE_API_PROXY_TARGET` | `http://127.0.0.1:8080` | адрес Go-бэкенда для dev-прокси |

В dev-режиме Vite проксирует `/api/*` и `/healthz` на `VITE_API_PROXY_TARGET`, чтобы не требовать CORS на бэкенде.

### API-клиент (`src/api/client.ts`)
- **`listExpenses(filters)`** → `GET /api/v1/expenses?user_id=...&category=...&from=...&to=...&limit=...&offset=...`
- **`createExpense(payload)`** → `POST /api/v1/expenses` (JSON-тело, возвращает созданную запись)
- **`replaceExpense(id, payload)`** → `PUT /api/v1/expenses/{id}` (полная замена)
- **`deleteExpense(id)`** → `DELETE /api/v1/expenses/{id}` (204 No Content)
- **`ApiError`**: кастомный класс с `status`, `details`, `raw` для обработки ошибок бэкенда (`400`, `404`, `422`, `429`, `500`)
- **`checkHealth()`** → `GET /healthz` (для индикатора живости, не используется в UI)

### Хук `useExpenses`
- **Состояние**: `expenses`, `loading`, `error`, `filters`, `page`, `hasMore`, `flash`
- **Действия**: `setFilters`, `resetFilters`, `setPage`, `refresh`, `remove`
- **Логика**:
  - При изменении `filters` или `page` → `listExpenses()` с новыми параметрами
  - `hasMore` = `data.length >= limit` (если бэкенд вернул меньше, значит это последняя страница)
  - `remove(id)` → `deleteExpense(id)` → `refresh()` → flash «Трата удалена»
  - При ошибке API → `error` с кнопкой «Повторить»

### Компоненты
- **`ConfirmDialog`**: модальное окно с фокусом на «Отмена», закрытие по Escape/клику на overlay, кнопка «Удалить» (destructive)
- **`ExpenseForm`**: форма с валидацией (user_id > 0, amount > 0, category обязательна, spent_at обязательна, comment ≤ 500 символов), переиспользуется для создания и редактирования
- **`ExpenseTable`**: адаптивная таблица (desktop → `<table>`, mobile → grid с `data-label`), кнопки «Изменить»/«Удалить»
- **`Filters`**: три поля (ID, категория, период), кнопка «Сбросить» (появляется при изменении)
- **`StatusViews`**: `Loading` (спиннер), `ErrorBanner` (красный баннер с retry), `EmptyState` (заглушка + CTA), `Flash` (всплывающее уведомление)

### Сборка и запуск
```bash
# 1. Установить зависимости
cd web
npm install

# 2. Запустить в dev-режиме (проксирует /api на Go-бэкенд)
npm run dev
# Откроется http://localhost:5173

# 3. Сборка для production
npm run build
# Результат в dist/

# 4. TypeScript-проверка без сборки
npm run typecheck
```

### Проверки
- ✅ `npm run build` — TypeScript компилируется без ошибок, Vite собирает `dist/`
- ✅ `npm run typecheck` — строгая проверка типов (`strict: true`, `noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`)
- ✅ `npm run dev -- --host` — dev-сервер на `:5173` с прокси на `:8080`

### Интеграция с бэкендом
- Бэкенд должен быть запущен на `:8080` (или указан в `VITE_API_PROXY_TARGET`)
- CORS на бэкенде не требуется (dev-прокси обходит)
- Для production: собрать `dist/` и раздавать через nginx/Caddy, либо встроить в Go-сервер (`http.FileServer`)

### Ограничения
- Нет авторизации (как и требуется)
- Нет графиков (только таблица)
- Нет сложной архитектуры (Zustand, Redux, React Query) — только локальное состояние в хуке
- Нет LLM-интеграции на фронте (бот работает отдельно в Telegram)
- Нет Dockerfile/docker-compose (см. Этап 6)

## Этап 6 (выполнено): Docker Compose и production-сборка

Весь сервис запускается одной командой `docker compose up --build -d`.

### Архитектура развёртывания
- **`api`** — Go REST API (бинарник `server` из образа `expense-bot/app`). Порт не публикуется; доступ через `web`.
- **`bot`** — Telegram-бот (бинарник `bot` из того же образа `expense-bot/app`).
- **`web`** — Nginx с собранным React-приложением (`expense-bot/web`); проксирует `/api/` и `/healthz` на `api:8080`.
- **`expense-data`** — named volume, монтируется в `/data` zarówno `api`, так и `bot`; файл SQLite `/data/expenses.db` общий.
- Сервисы связаны внутренней Docker-сетью (`expense-bot_default`).
- Перезапуск: `unless-stopped`; у `api` healthcheck на `GET /healthz`; `web` стартует после `api` (`condition: service_healthy`).

### Файлы этапа 6
- `Dockerfile` — многостейджевый: сборка на `golang:1.26-alpine` (CGO_ENABLED=0), runtime на `alpine:3.21` с `ca-certificates` + `wget`, без исходников/Go toolchain, пользователь `expensebot` (uid 1000), WORKDIR `/data`.
- `web/Dockerfile` — сборка на `node:24-alpine` (`npm ci`, `npm run build` = `tsc -b && vite build`), раздача через `nginx:1.29-alpine`, `nginx.conf` с SPA-фоллбэком и прокси `/api`, `/healthz`.
- `web/nginx.conf` — статика из `/usr/share/nginx/html`, `try_files ... /index.html`, прокси на `api:8080` без изменения URI.
- `compose.yaml` — определение сервисов, volume, сети, переменных.
- `.env.example` — шаблон переменных окружения (без секретов); `.env` (локальный, в `.gitignore`) — реальные значения.
- `.dockerignore`, `web/.dockerignore` — исключают исходники, `node_modules`, `dist`, `.env`, БД из контекста сборки.

### Изменения в коде (минимум, обратная совместимость)
- `config.Config` получило поле `DBPath` (env `DB_PATH`, по умолчанию `expenses.db`); `cmd/bot/main.go` использует `cfg.DBPath` вместо жёсткого `"expenses.db"`. `ServerConfig.DBPath` уже существовал.
- `internal/repository/sqlite/repository.go`: подключение к одному файлу SQLite из двух процессов — `_journal_mode=WAL`, `_busy_timeout=5000`, `_foreign_keys=1`, `_txlock=IMMEDIATE`, `SetMaxOpenConns(1)`. Схема `CREATE TABLE IF NOT EXISTS` безопасна при одновременном старте.

### Переменные окружения (`.env`)
См. `.env.example`. Обязательно заполнить `TELEGRAM_BOT_TOKEN` (без него бот не стартует). Пример минимального `.env`:
```
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
LLM_PRIMARY_URL=https://api.groq.com/openai/v1/chat/completions
LLM_PRIMARY_KEY=...
LLM_PRIMARY_MODEL=llama-3.3-70b-versatile
DB_PATH=/data/expenses.db
WEB_PORT=8080
```

### Как запустить
```bash
# 1. Скопировать шаблон и заполнить токены
cp .env.example .env
# (отредактировать .env: TELEGRAM_BOT_TOKEN, LLM ключи)

# 2. Собрать и запустить все сервисы в фоне
docker compose up --build -d

# 3. Открыть веб-панель: http://localhost:8080 (порт меняется переменной WEB_PORT)
```

### Логи и управление
```bash
docker compose logs -f        # логи всех сервисов
docker compose logs -f bot    # логи только бота
docker compose logs -f api    # логи только API
docker compose ps             # статус сервисов
docker compose stop           # остановить
docker compose start          # запустить снова
docker compose down           # остановить и удалить контейнеры
```

### Данные между перезапусками
SQLite-файл хранится в named volume `expense-data` (`/data/expenses.db`); данные сохраняются после `docker compose down` и пересборки.

Полное удаление (включая базу):
```bash
docker compose down -v   # ВНИМАНИЕ: удаляет volume expense-data вместе с expenses.db
```

### Пересборка отдельного сервиса
```bash
docker compose build api      # пересобрать образ сервиса
docker compose up --build -d api bot web
```

### Проверка работоспособности
- API: `curl http://localhost:8080/healthz` → `{"status":"ok"}` (или через Nginx, тот же путь)
- Веб-панель: http://localhost:8080
- Telegram-бот: написать боту «кофе 350» (нужен действительный `TELEGRAM_BOT_TOKEN`)
- БД общая: записи, сделанные ботом, видны в веб-панели и наоборот (один volume `/data`)

### Ограничения
- Docker Desktop daemon в текущей среде не запущен, поэтому `docker compose build/up` не проверялись вживую (проверен только `docker compose config`).
- Для работы бота нужен реальный `TELEGRAM_BOT_TOKEN`; для ответов ИИ — доступ к LLM (primary/backup).


