package main

import (
	"log"

	"expense-bot/config"
	"expense-bot/internal/bot"

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

	ebot := bot.NewEchoBot(api, cfg.Debug)
	if err := ebot.Run(); err != nil {
		log.Fatalf("bot run error: %v", err)
	}
}