# ============================================================
# Этап 1: сборка Go-бинарников (бот + REST API)
# ============================================================
FROM golang:1.26-alpine AS build

WORKDIR /app

# Кэшируем загрузку модулей отдельно от исходников.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Статически компонуем бинарники без CGO (modernc.org/sqlite — чистый Go).
# alpine + CGO_ENABLED=0 даёт runtime-образ без gcc и системного SQLite.
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/bot ./cmd/bot \
	&& CGO_ENABLED=0 GOOS=linux go build -o /bin/server ./cmd/server

# ============================================================
# Этап 2: минимальный runtime-образ на Alpine
# ============================================================
FROM alpine:3.21 AS runtime

# CA-сертификаты нужны для HTTPS-запросов к LLM API;
# wget — для healthcheck (/healthz). Больше ничего не требуется.
RUN apk add --no-cache ca-certificates wget

# Непривилегированный пользователь для запуска приложения.
RUN addgroup -g 1000 expensebot \
	&& adduser -u 1000 -G expensebot -D expensebot

# Директория для volume с SQLite-файлом; права отдаём приложению.
RUN mkdir -p /data && chown -R expensebot:expensebot /data
WORKDIR /data

COPY --from=build /bin/bot  /bin/bot
COPY --from=build /bin/server /bin/server

USER expensebot

EXPOSE 8080

# Логи пишутся в stdout/stderr; корректная обработка SIGTERM/SIGINT
# реализована в приложении (signal.NotifyContext + graceful shutdown).
ENTRYPOINT []
CMD ["server"]
