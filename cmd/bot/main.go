package main

import (
	"log"

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

	// Бот
	b := bot.NewEchoBot(api, cfg.Debug, svc)

	// Запуск бота (блокирующий вызов)
	if err := b.Run(); err != nil {
		log.Fatalf("bot run error: %v", err)
	}
}