// migrate 是一个极简迁移执行器：按文件名顺序执行 migrations/ 下尚未应用的 .sql 文件。
// MVP 阶段不引入 golang-migrate，文件格式为 NNNN_描述.sql。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Archy-yang/tongpin/backend/internal/config"
	"github.com/Archy-yang/tongpin/backend/internal/platform/database"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		log.Fatalf("ensure schema_migrations: %v", err)
	}

	applied := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		log.Fatalf("list applied: %v", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			log.Fatalf("scan applied: %v", err)
		}
		applied[v] = true
	}
	rows.Close()

	entries, err := os.ReadDir("migrations")
	if err != nil {
		log.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)

	for _, name := range files {
		if applied[strings.TrimSuffix(name, ".sql")] {
			continue
		}
		sqlBytes, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			log.Fatalf("read %s: %v", name, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatalf("begin tx: %v", err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("apply %s: %v", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			strings.TrimSuffix(name, ".sql")); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("record %s: %v", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("commit %s: %v", name, err)
		}
		fmt.Printf("applied %s\n", name)
	}
	fmt.Println("migrations up to date")
}
