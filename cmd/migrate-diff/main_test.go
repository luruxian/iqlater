// 职责: 验证 migrate-diff 对迁移目录 atlas.sum 的校验能识别被篡改的迁移文件。
// 关联文件:
//   cmd/migrate-diff/main.go

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ariga.io/atlas/sql/migrate"
	"github.com/stretchr/testify/require"
)

func TestValidateDirRejectsTamperedFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "20261128000000_baseline.sql")
	require.NoError(t, os.WriteFile(file, []byte("CREATE TABLE t (id bigint);\n"), 0o644))

	local, err := migrate.NewLocalDir(dir)
	require.NoError(t, err)
	files, err := local.Files()
	require.NoError(t, err)
	sum, err := migrate.NewHashFile(files)
	require.NoError(t, err)
	require.NoError(t, migrate.WriteSumFile(local, sum))

	require.NoError(t, validateDir(dir))

	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("DROP TABLE t;\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	err = validateDir(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, migrate.ErrChecksumMismatch), "got %v", err)
}
