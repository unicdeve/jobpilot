// Command migrate is a development helper for creating and applying the
// SQLite migrations embedded in internal/adapters/secondary/sqlite. It is a
// thin wrapper around the same goose library calls the application itself
// uses (DB.InitializeSchema) — see the `make migrate-*` targets — and is not
// part of the shipped application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

// migrationsDir must match the embed pattern in
// internal/adapters/secondary/sqlite/migrations.go.
const migrationsDir = "internal/adapters/secondary/sqlite/migrations"

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database file for up/down/status "+
		"(defaults to the same path the app itself uses, via system.DefaultDBPath)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s -db <path> up|down|status\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s create <name>\n", os.Args[0])
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()

	switch cmd := args[0]; cmd {
	case "create":
		if len(args) != 2 {
			flag.Usage()
			os.Exit(2)
		}
		// db is unused by goose.Create unless sequential (non-timestamp)
		// versioning is enabled, which this project doesn't use.
		if err := goose.Create(nil, migrationsDir, args[1], "sql"); err != nil {
			log.Fatalf("failed to create migration: %v", err)
		}

	case "up", "down", "status":
		resolvedPath := *dbPath
		if resolvedPath == "" {
			var err error
			resolvedPath, err = system.DefaultDBPath()
			if err != nil {
				log.Fatalf("failed to resolve default database path: %v", err)
			}
			fmt.Printf("-db not given, using default: %s\n", resolvedPath)
		}
		db, err := sqlite.NewDB(resolvedPath)
		if err != nil {
			log.Fatalf("failed to open database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Printf("error closing database: %v", err)
			}
		}()

		switch cmd {
		case "up":
			if err := db.InitializeSchema(ctx); err != nil {
				log.Fatalf("migrate up failed: %v", err)
			}
			fmt.Println("migrations applied")

		case "down":
			result, err := db.MigrateDown(ctx)
			if err != nil {
				log.Fatalf("migrate down failed: %v", err)
			}
			fmt.Printf("rolled back: %s\n", result.Source.Path)

		case "status":
			statuses, err := db.MigrationStatus(ctx)
			if err != nil {
				log.Fatalf("migrate status failed: %v", err)
			}
			for _, s := range statuses {
				applied := "pending"
				if s.State == goose.StateApplied {
					applied = s.AppliedAt.Format("2006-01-02 15:04:05")
				}
				path := s.Source.Path
				if path == "" {
					path = "(Go migration, see migrations_go.go)"
				}
				fmt.Printf("%-3d %-50s %s\n", s.Source.Version, path, applied)
			}
		}

	default:
		flag.Usage()
		os.Exit(2)
	}
}
