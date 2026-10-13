package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/dbtest"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestUsageAndUnknownCommands(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"help"}, env(nil), &out); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"serve", "migrate", "seed"} {
		if !strings.Contains(out.String(), cmd) {
			t.Errorf("usage does not mention %s:\n%s", cmd, out.String())
		}
	}
	if err := run(context.Background(), nil, env(nil), &out); err == nil {
		t.Error("no command should be an error")
	}
	if err := run(context.Background(), []string{"launch"}, env(map[string]string{"SLOTBOOK_DATABASE_URL": "postgres://x"}), &out); err == nil {
		t.Error("an unknown command should be an error")
	}
}

func TestServeRefusesAWeakAdminToken(t *testing.T) {
	err := run(context.Background(), []string{"serve"}, env(map[string]string{
		"SLOTBOOK_DATABASE_URL": "postgres://127.0.0.1:1/none",
		"SLOTBOOK_ADMIN_TOKEN":  "short",
	}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "SLOTBOOK_ADMIN_TOKEN") {
		t.Fatalf("got %v, want a complaint about the admin token", err)
	}
}

func TestMigrateAndSeedAnEmptyDatabase(t *testing.T) {
	url := dbtest.EmptySchema(t)
	vars := env(map[string]string{"SLOTBOOK_DATABASE_URL": url, "SLOTBOOK_LOG_LEVEL": "error"})
	ctx := context.Background()
	var out bytes.Buffer
	for _, args := range [][]string{{"migrate"}, {"migrate"}, {"seed", "../../seed/demo.json"}, {"seed", "../../seed/demo.json"}} {
		if err := run(ctx, args, vars, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var locations, staff int
	if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM locations), (SELECT count(*) FROM staff)`).Scan(&locations, &staff); err != nil {
		t.Fatal(err)
	}
	if locations != 2 || staff != 6 {
		t.Fatalf("%d locations and %d staff after seeding twice", locations, staff)
	}
}
