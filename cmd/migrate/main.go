package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/cyxc1124/osspilot-tenant-api/internal/config"
	"github.com/cyxc1124/osspilot-tenant-api/internal/logx"
	"github.com/cyxc1124/osspilot-tenant-api/migrations"
)

func main() {
	logx.Setup("osspilot-tenant-api")
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		slog.Error("ping db", "err", err)
		os.Exit(1)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("dialect", "err", err)
		os.Exit(1)
	}

	args := os.Args[1:]
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}

	if err := goose.RunContext(context.Background(), command, db, "."); err != nil {
		fmt.Fprintf(os.Stderr, "migrate %s: %v\n", command, err)
		os.Exit(1)
	}
}
