package config

import (
	"os"
	"strconv"
	"time"
)

// Config хранит настройки приложения.
type Config struct {
	TelegramBotToken string
	Debug            bool

	// LLM настройки
	LLMPrimaryURL      string
	LLMPrimaryKey      string
	LLMBackupURL       string
	LLMBackupKey       string
	LLMTimeoutSeconds  int
	LLMMaxRetries      int
	LLMRetryDelaySec   int
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

	timeout := 30
	if v := os.Getenv("LLM_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	maxRetries := 2
	if v := os.Getenv("LLM_MAX_RETRIES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			maxRetries = parsed
		}
	}

	retryDelay := 2
	if v := os.Getenv("LLM_RETRY_DELAY_SEC"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			retryDelay = parsed
		}
	}

	return &Config{
		TelegramBotToken:   token,
		Debug:              debug,
		LLMPrimaryURL:      os.Getenv("LLM_PRIMARY_URL"),
		LLMPrimaryKey:      os.Getenv("LLM_PRIMARY_KEY"),
		LLMBackupURL:       os.Getenv("LLM_BACKUP_URL"),
		LLMBackupKey:       os.Getenv("LLM_BACKUP_KEY"),
		LLMTimeoutSeconds:  timeout,
		LLMMaxRetries:      maxRetries,
		LLMRetryDelaySec:   retryDelay,
	}, nil
}

// LLMTimeout возвращает таймаут запроса к LLM.
func (c *Config) LLMTimeout() time.Duration {
	return time.Duration(c.LLMTimeoutSeconds) * time.Second
}

// LLRetryDelay возвращает задержка между повторами.
func (c *Config) LLRetryDelay() time.Duration {
	return time.Duration(c.LLMRetryDelaySec) * time.Second
}