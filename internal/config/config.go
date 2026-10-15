// Package config reads the service configuration from the environment and
// holds the defaults for per-location booking settings.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/sinnercode228/slotbook/internal/ratelimit"
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

// DefaultHoldLimit lets one client address make ten holds at once and two
// a minute after that.
var DefaultHoldLimit = ratelimit.Limit{Count: 2, Per: time.Minute, Burst: 10}

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
	HoldLimit        ratelimit.Limit
	TrustedProxies   []netip.Prefix
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
	if cfg.HoldLimit, err = holdLimit(getenv); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxies, err = prefixList(getenv, "SLOTBOOK_TRUSTED_PROXIES"); err != nil {
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

// holdLimit reads SLOTBOOK_HOLD_RATE, a count per period ("10/min", "1/s",
// "120/h", "5/10m") or "0" for no limit, and SLOTBOOK_HOLD_BURST.
func holdLimit(getenv func(string) string) (ratelimit.Limit, error) {
	limit := DefaultHoldLimit
	if v := getenv("SLOTBOOK_HOLD_BURST"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			return ratelimit.Limit{}, errors.New("config: SLOTBOOK_HOLD_BURST must be a positive integer (SLOTBOOK_HOLD_RATE=0 turns the limit off)")
		}
		limit.Burst = n
	}
	v := strings.TrimSpace(getenv("SLOTBOOK_HOLD_RATE"))
	switch v {
	case "":
		return limit, nil
	case "0":
		return ratelimit.Limit{}, nil
	}
	countText, periodText, _ := strings.Cut(v, "/")
	count, err := strconv.Atoi(strings.TrimSpace(countText))
	if err != nil || count < 0 || periodText == "" {
		return ratelimit.Limit{}, fmt.Errorf("config: SLOTBOOK_HOLD_RATE %q must be a count per period, such as 10/min, or 0", v)
	}
	per, err := ratePeriod(strings.TrimSpace(periodText))
	if err != nil {
		return ratelimit.Limit{}, fmt.Errorf("config: SLOTBOOK_HOLD_RATE: %w", err)
	}
	if count == 0 {
		return ratelimit.Limit{}, nil
	}
	limit.Count, limit.Per = count, per
	return limit, nil
}

func ratePeriod(v string) (time.Duration, error) {
	switch v {
	case "s":
		return time.Second, nil
	case "m", "min":
		return time.Minute, nil
	case "h":
		return time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("period %q must be s, min, h or a positive duration such as 10m", v)
	}
	return d, nil
}

// prefixList reads comma-separated CIDR prefixes; a bare address stands for
// itself alone.
func prefixList(getenv func(string) string, key string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range splitList(getenv(key)) {
		p, err := netip.ParsePrefix(item)
		if err != nil {
			a, addrErr := netip.ParseAddr(item)
			if addrErr != nil {
				return nil, fmt.Errorf("config: %s: %q is neither a CIDR prefix nor an IP address", key, item)
			}
			a = a.WithZone("").Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
