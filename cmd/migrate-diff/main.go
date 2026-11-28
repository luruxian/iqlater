// 职责: 对比 Ent schema 与 PostgreSQL 版本化迁移目录，生成新的 Atlas 迁移文件，或在 -check 时确认没有遗漏的变更。
// 关联文件:
//   internal/server/db/migration.go
//   internal/ent/migrate/schema.go
//   internal/ent/migrate/versioned/postgres/atlas.sum
//   scripts/migration/versioned.sh
// 注意事项:
//   迁移选项必须来自 db.SchemaMigrateOptions，保证生成的 SQL 与 auto 模式的 Schema.Create 结构一致。
//   -dev-url 必须是空库并带 search_path=public，否则 SQL 会带 schema 前缀或 replay 失败。

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/looplj/axonhub/internal/ent/migrate"
	"github.com/looplj/axonhub/internal/server/db"
)

const defaultDir = "internal/ent/migrate/versioned/postgres"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stderr))
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate-diff", flag.ContinueOnError)
	fs.SetOutput(stderr)

	devURL := fs.String("dev-url", "", "empty PostgreSQL dev database DSN, must include search_path=public")
	name := fs.String("name", "", "migration name")
	dir := fs.String("dir", defaultDir, "migration directory")
	check := fs.Bool("check", false, "fail if the schema has changes not captured in the migration directory")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *devURL == "" || (*name == "" && !*check) {
		fmt.Fprintln(stderr, "usage: migrate-diff -dev-url <postgres DSN> -name <name> [-dir <path>] [-check]")
		return 2
	}

	if *check {
		return runCheck(ctx, *devURL, *dir, *name, stderr)
	}

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "create migration directory: %v\n", err)
		return 1
	}

	if err := diff(ctx, *devURL, *dir, *name); err != nil {
		fmt.Fprintf(stderr, "generate migration: %v\n", err)
		return 1
	}

	return 0
}

// runCheck 校验 atlas.sum，再在目录副本上重新 diff；产生新文件即说明 schema 变更未生成迁移。
func runCheck(ctx context.Context, devURL, dir, name string, stderr io.Writer) int {
	if name == "" {
		name = "check"
	}

	if err := validateDir(dir); err != nil {
		fmt.Fprintf(stderr, "invalid migration directory %s: %v\n", dir, err)
		return 1
	}

	tmp, err := os.MkdirTemp("", "migrate-diff-check-")
	if err != nil {
		fmt.Fprintf(stderr, "create temp directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	if err := os.CopyFS(tmp, os.DirFS(dir)); err != nil {
		fmt.Fprintf(stderr, "copy migration directory: %v\n", err)
		return 1
	}

	before, err := countFiles(tmp)
	if err != nil {
		fmt.Fprintf(stderr, "read migration directory: %v\n", err)
		return 1
	}

	if err := diff(ctx, devURL, tmp, name); err != nil {
		fmt.Fprintf(stderr, "diff migration: %v\n", err)
		return 1
	}

	after, err := countFiles(tmp)
	if err != nil {
		fmt.Fprintf(stderr, "read migration directory: %v\n", err)
		return 1
	}

	if after != before {
		fmt.Fprintln(stderr, "schema changes not captured in migrations; run: make migrate-diff name=<name>")
		return 1
	}

	return 0
}

// validateDir 校验迁移目录与 atlas.sum 一致。
func validateDir(path string) error {
	dir, err := atlasmigrate.NewLocalDir(path)
	if err != nil {
		return err
	}

	return atlasmigrate.Validate(dir)
}

// diff 在 dev 库上回放迁移目录，与 Ent schema 对比后把变更写成新的迁移文件。
func diff(ctx context.Context, devURL, path, name string) error {
	dir, err := atlasmigrate.NewLocalDir(path)
	if err != nil {
		return err
	}

	sqlDB, err := sql.Open("pgx", devURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	opts := append(db.SchemaMigrateOptions(),
		schema.WithDir(dir),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(dialect.Postgres),
		schema.WithFormatter(atlasmigrate.DefaultFormatter),
	)

	m, err := schema.NewMigrate(entsql.OpenDB(dialect.Postgres, sqlDB), opts...)
	if err != nil {
		return err
	}

	return m.NamedDiff(ctx, name, migrate.Tables...)
}

func countFiles(path string) (int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			n++
		}
	}

	return n, nil
}
