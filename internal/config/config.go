// Package config загружает конфигурацию сервиса из переменных окружения.
// Секреты (строка подключения к БД) передаются только через окружение —
// в репозиторий попадает лишь .env.example (требование А5).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config — параметры запуска сервиса.
type Config struct {
	ListenAddr     string        // адрес HTTP-сервера
	DatabaseURL    string        // строка подключения PostgreSQL (секрет)
	ExchangeAPIURL string        // базовый URL внешнего API курсов
	HTTPTimeout    time.Duration // таймаут запросов к внешнему API
	RatesTTL       time.Duration // время жизни кэша курсов
	LogLevel       string        // уровень slog: debug|info|warn|error
}

// Load читает переменные окружения, подставляя разумные значения по умолчанию.
func Load() Config {
	return Config{
		ListenAddr: env("FX_LISTEN_ADDR", ":8080"),
		DatabaseURL: env(
			"FX_DATABASE_URL",
			"postgres://currency:currency@localhost:5432/currency?sslmode=disable",
		),
		ExchangeAPIURL: env("FX_EXCHANGE_API_URL", "https://open.er-api.com/v6/latest"),
		HTTPTimeout:    envDuration("FX_HTTP_TIMEOUT", 5*time.Second),
		RatesTTL:       envDuration("FX_RATES_TTL", time.Hour),
		LogLevel:       env("FX_LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds < 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

// String возвращает человекочитаемое представление без секрета подключения к БД.
func (c Config) String() string {
	return fmt.Sprintf(
		"{listen:%s db:%s api:%s ttl:%s log:%s}",
		c.ListenAddr, "****", c.ExchangeAPIURL, c.RatesTTL, c.LogLevel,
	)
}
