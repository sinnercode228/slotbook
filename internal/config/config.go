// Package config reads the service configuration from the environment and
// holds the defaults for per-location booking settings.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// LocationSettings are the booking rules of one location.
type LocationSettings struct {
	SlotStep     time.Duration
	MinNotice    time.Duration
	HorizonDays  int
	HoldTTL      time.Duration
	CancelCutoff time.Duration
}

// LocationDefaults apply to every setting a location leaves NULL.
var LocationDefaults = LocationSettings{
	SlotStep:     15 * time.Minute,
	MinNotice:    2 * time.Hour,
	HorizonDays:  60,
	HoldTTL:      10 * time.Minute,
	CancelCutoff: 3 * time.Hour,
}

// MinAdminTokenLength is enforced unless SLOTBOOK_DEV is set.
const MinAdminTokenLength = 32

type Config struct {
	DatabaseURL      string
	Addr             string
	AdminToken       string
	Dev              bool
	CORSOrigins      []string
	TelegramBotToken string
	TelegramAPIURL   string
	StaticDir        string
	LogLevel         slog.Level
	WorkerInterval   time.Duration
	SweepInterval    time.Duration
}

// Load builds a Config from getenv (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:      getenv("SLOTBOOK_DATABASE_URL"),
		Addr:             withDefault(getenv("SLOTBOOK_ADDR"), ":8080"),
		AdminToken:       getenv("SLOTBOOK_ADMIN_TOKEN"),
		CORSOrigins:      splitList(getenv("SLOTBOOK_CORS_ORIGINS")),
		TelegramBotToken: getenv("SLOTBOOK_TELEGRAM_BOT_TOKEN"),
		TelegramAPIURL:   strings.TrimRight(withDefault(getenv("SLOTBOOK_TELEGRAM_API_URL"), "https://api.telegram.org"), "/"),
		StaticDir:        getenv("SLOTBOOK_STATIC_DIR"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("config: SLOTBOOK_DATABASE_URL is required")
	}
	var err error
	if v := getenv("SLOTBOOK_DEV"); v != "" {
		if cfg.Dev, err = strconv.ParseBool(v); err != nil {
			return Config{}, fmt.Errorf("config: SLOTBOOK_DEV: %w", err)
		}
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(withDefault(getenv("SLOTBOOK_LOG_LEVEL"), "info"))); err != nil {
		return Config{}, fmt.Errorf("config: SLOTBOOK_LOG_LEVEL: %w", err)
	}
	if cfg.WorkerInterval, err = positiveDuration(getenv, "SLOTBOOK_WORKER_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.SweepInterval, err = positiveDuration(getenv, "SLOTBOOK_SWEEP_INTERVAL", 30*time.Second); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// CheckAdminToken refuses a missing or short admin token; dev mode only
// lifts the length requirement.
func (c Config) CheckAdminToken() error {
	if c.AdminToken == "" {
		return errors.New("config: SLOTBOOK_ADMIN_TOKEN is required")
	}
	if len(c.AdminToken) < MinAdminTokenLength && !c.Dev {
		return fmt.Errorf("config: SLOTBOOK_ADMIN_TOKEN must be at least %d characters (set SLOTBOOK_DEV=true to allow a short one locally)", MinAdminTokenLength)
	}
	return nil
}

func withDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func positiveDuration(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", key)
	}
	return d, nil
}
