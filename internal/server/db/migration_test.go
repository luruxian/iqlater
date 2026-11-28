// 职责: 校验 migration_mode 对启动迁移步骤的取舍，以及 Ent schema 迁移选项的单一来源。
// 关联文件:
//   internal/server/db/migration.go
//   internal/server/db/config.go

package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationSteps(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantSchema bool
		wantData   bool
		wantErr    string
	}{
		{name: "unset", cfg: Config{}, wantSchema: true, wantData: true},
		{name: "auto", cfg: Config{MigrationMode: "auto"}, wantSchema: true, wantData: true},
		{name: "versioned", cfg: Config{MigrationMode: "versioned"}, wantSchema: false, wantData: true},
		{name: "disabled", cfg: Config{DisableAutoMigration: true}},
		{name: "disabled versioned", cfg: Config{DisableAutoMigration: true, MigrationMode: "versioned"}},
		{name: "bogus", cfg: Config{MigrationMode: "bogus"}, wantErr: `invalid db.migration_mode "bogus"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSchema, runData, err := migrationSteps(tt.cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantSchema, runSchema)
			require.Equal(t, tt.wantData, runData)
		})
	}
}

func TestSchemaMigrateOptionsCount(t *testing.T) {
	require.Len(t, SchemaMigrateOptions(), 6)
}
