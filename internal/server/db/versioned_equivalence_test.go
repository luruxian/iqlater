// 职责: 验证 PostgreSQL 版本化迁移目录建出的结构与 auto 模式 Schema.Create 建出的结构完全一致。
// 关联文件:
//   internal/server/db/migration.go
//   internal/ent/migrate/versioned/postgres/atlas.sum
//   cmd/migrate-diff/main.go
// 注意事项:
//   需要 AXONHUB_TEST_PG_DSN 指向可建删 schema 的 PostgreSQL，未设置时跳过。
//   两个 schema 名不同，比较前需把名称对齐，否则 atlas SchemaDiff 直接报错。

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/postgres"
	aschema "ariga.io/atlas/sql/schema"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
)

const versionedPostgresDir = "../../ent/migrate/versioned/postgres"

// TestVersionedMigrationsMatchAutoMigrate applies every versioned migration in
// schema va_* and runs auto-migrate Schema.Create in schema vb_*, then requires
// an empty atlas SchemaDiff between the two.
//
//	AXONHUB_TEST_PG_DSN="postgres://postgres:pass@127.0.0.1:55432/postgres?sslmode=disable" \
//	  go test ./internal/server/db -run TestVersionedMigrationsMatchAutoMigrate -v -count=1
func TestVersionedMigrationsMatchAutoMigrate(t *testing.T) {
	dsn := os.Getenv("AXONHUB_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("AXONHUB_TEST_PG_DSN not set; skipping versioned/auto migration equivalence")
	}

	ctx := context.Background()

	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)

	adminDB := stdlib.OpenDB(*config.Copy())
	require.NoError(t, adminDB.PingContext(ctx))
	t.Cleanup(func() { _ = adminDB.Close() })

	suffix := uuid.NewString()[:8]
	versioned := openSchemaDB(t, adminDB, config, "va_"+suffix)
	auto := openSchemaDB(t, adminDB, config, "vb_"+suffix)

	dir, err := atlasmigrate.NewLocalDir(versionedPostgresDir)
	require.NoError(t, err)
	files, err := dir.Files()
	require.NoError(t, err)
	require.NotEmpty(t, files, "no versioned migrations found in %s", versionedPostgresDir)

	for _, f := range files {
		stmts, err := f.Stmts()
		require.NoError(t, err, f.Name())

		for _, stmt := range stmts {
			_, err := versioned.db.ExecContext(ctx, stmt)
			require.NoError(t, err, "%s: %s", f.Name(), stmt)
		}
	}

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, auto.db)))
	require.NoError(t, client.Schema.Create(ctx, SchemaMigrateOptions()...))

	drv, err := postgres.Open(adminDB)
	require.NoError(t, err)

	got, err := drv.InspectSchema(ctx, versioned.name, nil)
	require.NoError(t, err)
	want, err := drv.InspectSchema(ctx, auto.name, nil)
	require.NoError(t, err)
	require.NotEmpty(t, want.Tables, "auto-migrate created no tables in %s", auto.name)

	// SchemaDiff refuses to compare schemas with different names.
	got.Name = want.Name

	changes, err := drv.SchemaDiff(got, want)
	require.NoError(t, err)

	for _, c := range changes {
		t.Logf("diff: %s", describeChange(c))
	}

	require.Empty(t, changes)
}

type schemaDB struct {
	name string
	db   *sql.DB
}

// openSchemaDB creates a throwaway schema and returns a connection whose
// search_path points only at it.
func openSchemaDB(t *testing.T, adminDB *sql.DB, base *pgx.ConnConfig, name string) schemaDB {
	t.Helper()
	ctx := context.Background()

	_, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %q", name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %q CASCADE", name))
	})

	config := base.Copy()
	config.RuntimeParams["search_path"] = name

	db := stdlib.OpenDB(*config)
	require.NoError(t, db.PingContext(ctx))
	t.Cleanup(func() { _ = db.Close() })

	return schemaDB{name: name, db: db}
}

// describeChange prints a change's type and table name for failure output.
func describeChange(c aschema.Change) string {
	switch v := c.(type) {
	case *aschema.AddTable:
		return fmt.Sprintf("%T %s", v, v.T.Name)
	case *aschema.DropTable:
		return fmt.Sprintf("%T %s", v, v.T.Name)
	case *aschema.ModifyTable:
		inner := make([]string, 0, len(v.Changes))
		for _, ic := range v.Changes {
			inner = append(inner, fmt.Sprintf("%T", ic))
		}

		return fmt.Sprintf("%T %s %v", v, v.T.Name, inner)
	default:
		return fmt.Sprintf("%T", c)
	}
}
