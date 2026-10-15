// Command slotbook runs the booking service.
//
//	slotbook serve           API, notification worker and hold sweeper
//	slotbook migrate         apply database migrations
//	slotbook seed [file]     load a demo chain (default seed/demo.json)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	// Embedded zone data keeps slot times independent of the image's tzdata.
	_ "time/tzdata"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/config"
	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/httpapi"
	"github.com/sinnercode228/slotbook/internal/notify"
	"github.com/sinnercode228/slotbook/internal/seed"
)

const usage = `usage: slotbook <command>

commands:
  serve          run the HTTP API, the notification worker and the hold sweeper
  migrate        apply database migrations
  seed [file]    load a chain of locations, services and staff (default seed/demo.json)

Configuration comes from SLOTBOOK_* environment variables; see .env.example.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "slotbook:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return errors.New("no command given")
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return nil
	}
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	switch args[0] {
	case "serve":
		return serve(ctx, cfg, log)
	case "migrate":
		return migrate(ctx, cfg, log)
	case "seed":
		path := "seed/demo.json"
		if len(args) > 1 {
			path = args[1]
		}
		return seedChain(ctx, cfg, log, path)
	default:
		return fmt.Errorf("unknown command %q (try slotbook help)", args[0])
	}
}

func migrate(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	log.Info("migrations applied", "versions", applied)
	return nil
}

func seedChain(ctx context.Context, cfg config.Config, log *slog.Logger, path string) error {
	chain, err := seed.LoadFile(path)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	res, err := seed.Apply(ctx, pool, chain)
	if err != nil {
		return err
	}
	log.Info("seed loaded", "file", path, "locations", len(res.Locations), "skipped_existing", res.Skipped)
	return nil
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	if err := cfg.CheckAdminToken(); err != nil {
		return err
	}
	if cfg.Dev {
		log.Warn("SLOTBOOK_DEV is set: weak admin tokens are accepted")
	}
	if !cfg.HoldLimit.Enabled() {
		log.Warn("SLOTBOOK_HOLD_RATE is 0: POST /v1/holds has no rate limit")
	}
	if cfg.StaticDir != "" {
		if _, err := os.Stat(filepath.Join(cfg.StaticDir, "index.html")); err != nil {
			log.Warn("static directory has no index.html, serving the API only", "dir", cfg.StaticDir)
			cfg.StaticDir = ""
		}
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := clock.System{}
	cat := catalog.NewStore(pool, config.LocationDefaults)
	bookings := booking.NewManager(pool, cat, clk)

	sender := notify.Router{Fallback: notify.LogSender{Log: log}}
	if cfg.TelegramBotToken != "" {
		sender.Telegram = notify.NewTelegram(cfg.TelegramAPIURL, cfg.TelegramBotToken, &http.Client{Timeout: 15 * time.Second})
	}
	worker := notify.NewWorker(pool, sender, clk, log)

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.New(httpapi.Deps{
			DB:             pool,
			Catalog:        cat,
			Bookings:       bookings,
			Clock:          clk,
			Log:            log,
			AdminToken:     cfg.AdminToken,
			CORSOrigins:    cfg.CORSOrigins,
			StaticDir:      cfg.StaticDir,
			HoldLimit:      cfg.HoldLimit,
			TrustedProxies: cfg.TrustedProxies,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Background loops get their own context so they keep running while
	// in-flight HTTP requests drain, and stop right after.
	bgCtx, stopBackground := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		worker.Run(bgCtx, cfg.WorkerInterval)
	}()
	go func() {
		defer wg.Done()
		bookings.RunSweeper(bgCtx, cfg.SweepInterval, log)
	}()
	defer func() {
		stopBackground()
		wg.Wait()
	}()

	listenErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		listenErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-listenErr:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
