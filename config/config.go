package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config хранит настройки приложения.
type Config struct {
	TelegramBotToken string
	Debug            bool
	DBPath           string

	// LLM настройки
	LLMPrimaryURL     string
	LLMPrimaryKey     string
	LLMPrimaryModel   string
	LLMBackupURL      string
	LLMBackupKey      string
	LLMBackupModel    string
	LLMTimeoutSeconds int
	LLMMaxRetries     int
	LLMRetryDelaySec  int
	LLMMaxTokens      int
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

	maxTokens := 512
	if v := os.Getenv("LLM_MAX_TOKENS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxTokens = parsed
		}
	}

	dbPath := "expenses.db"
	if v := os.Getenv("DB_PATH"); v != "" {
		dbPath = v
	}

	return &Config{
		TelegramBotToken:  token,
		Debug:             debug,
		DBPath:            dbPath,
		LLMPrimaryURL:     os.Getenv("LLM_PRIMARY_URL"),
		LLMPrimaryKey:     os.Getenv("LLM_PRIMARY_KEY"),
		LLMPrimaryModel:   os.Getenv("LLM_PRIMARY_MODEL"),
		LLMBackupURL:      os.Getenv("LLM_BACKUP_URL"),
		LLMBackupKey:      os.Getenv("LLM_BACKUP_KEY"),
		LLMBackupModel:    os.Getenv("LLM_BACKUP_MODEL"),
		LLMTimeoutSeconds: timeout,
		LLMMaxRetries:     maxRetries,
		LLMRetryDelaySec:  retryDelay,
		LLMMaxTokens:      maxTokens,
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

// ServerConfig хранит настройки REST API сервера веб-панели.
type ServerConfig struct {
	Addr           string
	DBPath         string
	RateLimitRPS   float64
	RateLimitBurst int
	AllowedOrigins []string
	Debug          bool
}

// LoadServer загружает конфигурацию HTTP-сервера из переменных окружения.
// Токен Telegram здесь не нужен: сервер запускается отдельно от бота.
func LoadServer() *ServerConfig {
	addr := ":8080"
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		addr = v
	}

	dbPath := "expenses.db"
	if v := os.Getenv("DB_PATH"); v != "" {
		dbPath = v
	}

	rps := 10.0
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 {
			rps = parsed
		}
	}

	burst := 20
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			burst = parsed
		}
	}

	origins := []string{"*"}
	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		origins = origins[:0]
		for _, origin := range strings.Split(v, ",") {
			if origin = strings.TrimSpace(origin); origin != "" {
				origins = append(origins, origin)
			}
		}
	}

	debug := false
	if v := os.Getenv("DEBUG"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			debug = parsed
		}
	}

	return &ServerConfig{
		Addr:           addr,
		DBPath:         dbPath,
		RateLimitRPS:   rps,
		RateLimitBurst: burst,
		AllowedOrigins: origins,
		Debug:          debug,
	}
}
