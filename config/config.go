package config

import (
	"os"
	"strconv"
)

// Config хранит настройки приложения.
type Config struct {
	TelegramBotToken string
	Debug            bool
}

// Load загружает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, ErrEmptyBotToken
	}

	debug := false
	if v := os.Getenv("DEBUG"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			debug = parsed
		}
	}

	return &Config{
		TelegramBotToken: token,
		Debug:            debug,
	}, nil
}