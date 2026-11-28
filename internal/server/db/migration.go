// 职责: 定义 migration_mode 取值、启动迁移步骤的选择，以及 Ent schema 迁移选项的单一来源。
// 关联文件:
//   internal/server/db/config.go
//   internal/server/db/ent.go
//   internal/server/db/schema_default_filter.go
// 注意事项:
//   SchemaMigrateOptions 同时供启动迁移和 Atlas 迁移生成器使用，选项必须只在这里维护。

package db

import (
	"fmt"

	"entgo.io/ent/dialect/sql/schema"

	"github.com/looplj/axonhub/internal/ent/migrate"
	"github.com/looplj/axonhub/internal/ent/migrate/schemahook"
)

const (
	// MigrationModeAuto 启动时执行 Ent schema 迁移和数据迁移。
	MigrationModeAuto = "auto"
	// MigrationModeVersioned 启动时只执行数据迁移，schema 由 Atlas 版本化迁移管理。
	MigrationModeVersioned = "versioned"
)

// migrationSteps 根据配置决定启动时要执行的迁移步骤。
// disable_auto_migration 优先级最高，会跳过 schema 和数据迁移。
func migrationSteps(cfg Config) (runSchema bool, runData bool, err error) {
	switch cfg.MigrationMode {
	case "", MigrationModeAuto, MigrationModeVersioned:
	default:
		return false, false, fmt.Errorf("invalid db.migration_mode %q: want %q or %q", cfg.MigrationMode, MigrationModeAuto, MigrationModeVersioned)
	}

	if cfg.DisableAutoMigration {
		return false, false, nil
	}

	return cfg.MigrationMode != MigrationModeVersioned, true, nil
}

// SchemaMigrateOptions 返回传给 Ent Schema.Create 的全部迁移选项。
func SchemaMigrateOptions() []schema.MigrateOption {
	return []schema.MigrateOption{
		migrate.WithGlobalUniqueID(false),
		migrate.WithForeignKeys(false),
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
		schema.WithHooks(schemahook.V0_3_0),
		filterEquivalentDefaultChanges(),
	}
}
