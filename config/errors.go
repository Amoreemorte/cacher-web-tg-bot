package config

import "errors"

// ErrEmptyBotToken возвращается, когда токен Telegram-бота не задан.
var ErrEmptyBotToken = errors.New("TELEGRAM_BOT_TOKEN is not set")