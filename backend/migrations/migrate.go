package main

// Легкий migrator без внешних зависимостей: читает backend/migrations/*.sql
// по алфавиту, ведет таблицу schema_migrations(name). Идемпотентен.

import (
	"context"
	"embed"
	"fmt"
	"os"
	"sort"

	"github.com/jackc/pgx/v5"
)

//go:embed *.sql
var migrationFS embed.FS

func main() {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is empty")
		os.Exit(1)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW())`); err != nil {
		fmt.Fprintln(os.Stderr, "migrations table:", err)
		os.Exit(1)
	}

	entries, err := migrationFS.ReadDir(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, n := range names {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, n).Scan(&exists); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if exists {
			fmt.Println("skip", n)
			continue
		}
		sql, err := migrationFS.ReadFile(n)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			fmt.Fprintf(os.Stderr, "migration %s failed: %v\n", n, err)
			os.Exit(1)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, n); err != nil {
			tx.Rollback(ctx)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := tx.Commit(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("applied", n)
	}
}
