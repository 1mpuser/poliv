package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Загруженные настройки из окружения. Поля совпадают с переменными из docker-compose.
type Config struct {
	DatabaseURL   string
	JWTSecret     string
	JWTExpireDays int
	TZ            string
	Zone          *time.Location
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTExpireDays: 30,
		TZ:            valueOr(os.Getenv("TZ"), "Europe/Moscow"),
	}
	if dayStr := os.Getenv("JWT_EXPIRE_DAYS"); dayStr != "" {
		if n, err := strconv.Atoi(dayStr); err == nil {
			c.JWTExpireDays = n
		}
	}
	// pgx не понимает префикс postgresql+psycopg:// — принимаем оба
	c.DatabaseURL = normalizeDBURL(c.DatabaseURL)
	zone, err := time.LoadLocation(c.TZ)
	if err != nil {
		return nil, err
	}
	c.Zone = zone
	return c, nil
}

func normalizeDBURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "postgresql+psycopg://") {
		return "postgresql://" + strings.TrimPrefix(u, "postgresql+psycopg://")
	}
	if strings.HasPrefix(u, "postgresql+psycopg2://") {
		return "postgresql://" + strings.TrimPrefix(u, "postgresql+psycopg2://")
	}
	return u
}

func valueOr(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
