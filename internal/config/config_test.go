package config

import (
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/ratelimit"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SLOTBOOK_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.TelegramAPIURL != "https://api.telegram.org" || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.WorkerInterval != 5*time.Second || cfg.SweepInterval != 30*time.Second {
		t.Fatalf("unexpected intervals: %v %v", cfg.WorkerInterval, cfg.SweepInterval)
	}
	if len(cfg.CORSOrigins) != 0 || cfg.Dev {
		t.Fatalf("unexpected cors/dev: %+v", cfg)
	}
	if cfg.HoldLimit != (ratelimit.Limit{Count: 2, Per: time.Minute, Burst: 10}) || cfg.TrustedProxies != nil {
		t.Fatalf("unexpected hold limit %+v or trusted proxies %v", cfg.HoldLimit, cfg.TrustedProxies)
	}
}

func TestLoadReadsEverything(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"SLOTBOOK_DATABASE_URL":       "postgres://x",
		"SLOTBOOK_ADDR":               "127.0.0.1:9000",
		"SLOTBOOK_ADMIN_TOKEN":        "t",
		"SLOTBOOK_DEV":                "true",
		"SLOTBOOK_CORS_ORIGINS":       " https://a.example.com, https://b.example.com ,,",
		"SLOTBOOK_TELEGRAM_BOT_TOKEN": "123:abc",
		"SLOTBOOK_TELEGRAM_API_URL":   "http://127.0.0.1:1/",
		"SLOTBOOK_STATIC_DIR":         "web/dist",
		"SLOTBOOK_LOG_LEVEL":          "debug",
		"SLOTBOOK_WORKER_INTERVAL":    "2s",
		"SLOTBOOK_SWEEP_INTERVAL":     "1m",
		"SLOTBOOK_HOLD_RATE":          "30/h",
		"SLOTBOOK_HOLD_BURST":         "4",
		"SLOTBOOK_TRUSTED_PROXIES":    " 10.0.0.0/8, 2001:db8::/32 ,192.0.2.10,,10.1.2.3/8",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.CORSOrigins, []string{"https://a.example.com", "https://b.example.com"}) {
		t.Fatalf("cors origins %q", cfg.CORSOrigins)
	}
	if cfg.TelegramAPIURL != "http://127.0.0.1:1" {
		t.Fatalf("trailing slash kept: %q", cfg.TelegramAPIURL)
	}
	if !cfg.Dev || cfg.LogLevel != slog.LevelDebug || cfg.WorkerInterval != 2*time.Second || cfg.SweepInterval != time.Minute {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.HoldLimit != (ratelimit.Limit{Count: 30, Per: time.Hour, Burst: 4}) {
		t.Fatalf("hold limit %+v", cfg.HoldLimit)
	}
	wantProxies := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("192.0.2.10/32"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	if !slices.Equal(cfg.TrustedProxies, wantProxies) {
		t.Fatalf("trusted proxies %v, want %v", cfg.TrustedProxies, wantProxies)
	}
}

func TestHoldRateFormats(t *testing.T) {
	cases := map[string]ratelimit.Limit{
		"10/min":    {Count: 10, Per: time.Minute, Burst: 10},
		"10/m":      {Count: 10, Per: time.Minute, Burst: 10},
		"1/s":       {Count: 1, Per: time.Second, Burst: 10},
		"120/h":     {Count: 120, Per: time.Hour, Burst: 10},
		"5/10m":     {Count: 5, Per: 10 * time.Minute, Burst: 10},
		" 3 / min ": {Count: 3, Per: time.Minute, Burst: 10},
		"0":         {},
		"0/min":     {},
	}
	for v, want := range cases {
		cfg, err := Load(env(map[string]string{"SLOTBOOK_DATABASE_URL": "postgres://x", "SLOTBOOK_HOLD_RATE": v}))
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if cfg.HoldLimit != want {
			t.Errorf("%q: %+v, want %+v", v, cfg.HoldLimit, want)
		}
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"missing database url": {},
		"bad dev flag":         {"SLOTBOOK_DEV": "maybe"},
		"bad log level":        {"SLOTBOOK_LOG_LEVEL": "loud"},
		"bad interval":         {"SLOTBOOK_WORKER_INTERVAL": "soon"},
		"zero interval":        {"SLOTBOOK_SWEEP_INTERVAL": "0s"},
		"hold rate in words":   {"SLOTBOOK_HOLD_RATE": "ten/min"},
		"hold rate, no period": {"SLOTBOOK_HOLD_RATE": "10"},
		"unknown period":       {"SLOTBOOK_HOLD_RATE": "10/fortnight"},
		"negative hold rate":   {"SLOTBOOK_HOLD_RATE": "-1/min"},
		"zero period":          {"SLOTBOOK_HOLD_RATE": "10/0s"},
		"zero burst":           {"SLOTBOOK_HOLD_BURST": "0"},
		"burst in words":       {"SLOTBOOK_HOLD_BURST": "many"},
		"bad proxy prefix":     {"SLOTBOOK_TRUSTED_PROXIES": "10.0.0.0/33"},
		"proxy host name":      {"SLOTBOOK_TRUSTED_PROXIES": "proxy.internal"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if name != "missing database url" {
				vars["SLOTBOOK_DATABASE_URL"] = "postgres://x"
			}
			if _, err := Load(env(vars)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCheckAdminToken(t *testing.T) {
	strong := strings.Repeat("a", MinAdminTokenLength)
	if err := (Config{AdminToken: strong}).CheckAdminToken(); err != nil {
		t.Fatalf("strong token rejected: %v", err)
	}
	for _, weak := range []string{"", "short", strong[1:]} {
		if err := (Config{AdminToken: weak}).CheckAdminToken(); err == nil {
			t.Errorf("token of length %d accepted outside dev mode", len(weak))
		}
	}
	if err := (Config{AdminToken: "dev", Dev: true}).CheckAdminToken(); err != nil {
		t.Fatalf("dev mode should allow a short token: %v", err)
	}
	if err := (Config{Dev: true}).CheckAdminToken(); err == nil {
		t.Fatal("an empty token must be refused even in dev mode")
	}
}

func TestLocationDefaultsAreSane(t *testing.T) {
	d := LocationDefaults
	if d.SlotStep != 15*time.Minute || d.MinNotice != 2*time.Hour || d.HorizonDays != 60 ||
		d.HoldTTL != 10*time.Minute || d.CancelCutoff != 3*time.Hour {
		t.Fatalf("defaults changed: %+v", d)
	}
}
