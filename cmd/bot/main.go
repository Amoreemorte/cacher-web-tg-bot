package main

import (
	"context"
	"log"
	"time"

	"expense-bot/config"
	"expense-bot/internal/bot"
	"expense-bot/internal/llm"
	"expense-bot/internal/repository/sqlite"
	"expense-bot/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	api, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("telegram api error: %v", err)
	}

	api.Debug = cfg.Debug

	log.Printf("Authorized on account %s", api.Self.UserName)

	// Репозиторий (SQLite)
	repo, err := sqlite.NewSQLiteRepository("expenses.db")
	if err != nil {
		log.Fatalf("sqlite repository error: %v", err)
	}
	defer repo.Close()

	// LLM клиенты
	primary := llm.NewHTTPClient(cfg.LLMPrimaryURL, cfg.LLMPrimaryKey, cfg.LLMTimeout())
	var backup llm.LLMClient
	if cfg.LLMBackupURL != "" {
		backup = llm.NewHTTPClient(cfg.LLMBackupURL, cfg.LLMBackupKey, cfg.LLMTimeout())
	}

	// Фейловер LLM
	failover := llm.NewFailoverClient(primary, backup, cfg.LLMMaxRetries, cfg.LLRetryDelay())

	// Сервис
	svc := service.NewExpenseService(failover, repo, repo)

	// Процессор отложенных задач (повторная отправка в LLM при недоступности)
	processor := service.NewPendingTaskProcessor(failover, repo, repo, svc, 5, 30*time.Second)

	// Проверка отложенных задач при старте бота (холодный старт)
	if err := processor.ProcessPending(context.Background()); err != nil {
		log.Printf("pending processor initial check error: %v", err)
	}

	// Фоновая обработка отложенных задач (горячий запуск)
	go processor.Start(context.Background())

	// Бот
	b := bot.NewEchoBot(api, cfg.Debug, svc)

	// Запуск бота (блокирующий вызов)
	if err := b.Run(); err != nil {
		log.Fatalf("bot run error: %v", err)
	}
}