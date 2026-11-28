# iqlater 数据库迁移操作说明

iqlater 生产环境（PostgreSQL 18，Docker Compose，`deploy/iqlater/`）使用 Atlas 版本化迁移管理表结构。设计背景见 `docs/superpowers/specs/2026-11-28-iqlater-atlas-migrations-design.md`。

---

## 1. 两种迁移模式

由环境变量 `AXONHUB_DB_MIGRATION_MODE`（配置项 `db.migration_mode`）决定：

| 模式 | 启动时的行为 | 使用场景 |
|------|--------------|----------|
| `auto`（默认） | 清理 → `Schema.Create` → `datamigrate`，与上游一致 | 本地开发库、SQLite、测试 |
| `versioned` | 跳过清理和 `Schema.Create`，只执行 `datamigrate`。表结构由 `migrate` 服务（Atlas）负责 | iqlater 生产（compose 中已设置） |

- `disable_auto_migration: true` 优先级最高：全部跳过，包括 `datamigrate`。
- `migration_mode` 取值不是 `auto` 或 `versioned` 时启动失败。
- 迁移文件位于 `internal/ent/migrate/versioned/postgres/`（`<timestamp>_<name>.sql` + `atlas.sum`）。`internal/ent/migrate/migrations/` 是上游历史遗留，与此无关。
- 全新库上 axonhub 会打印 `system not initialized, skipping migration`，这是 `datamigrate` 的正常行为：系统初始化完成后，之后的升级才会执行 `datamigrate`。

## 2. 日常改表流程

1. 修改 `internal/ent/schema/`，运行 `make generate`。
2. 运行 `make migrate-diff name=<name>`，在迁移目录生成新的 SQL 文件并更新 `atlas.sum`。
   - 默认会启动一个临时的 `postgres:18` 容器并在结束后销毁，需要本机有 Docker。
   - 也可以用 `MIGRATE_DEV_URL` 指定一个空的 PostgreSQL 库，DSN 必须包含 `search_path=public`，例如 `postgres://postgres:pass@127.0.0.1:5432/dev?sslmode=disable&search_path=public`。
3. 人工审查生成的 SQL，重点看 `DROP`、类型变更、NOT NULL 新列。
   - 手工修改了生成但尚未提交的文件（例如补充 DDL 之前的清理 SQL，或加 `-- atlas:nolint` 指令）后，运行 `make migrate-hash` 刷新 `atlas.sum`。该命令使用固定版本的 Atlas 镜像，需要 Docker。
   - CI 的 `migrate lint` 会拒绝 `DROP COLUMN` / `DROP TABLE` 等破坏性变更。对已审查、确属预期的删除（expand-contract 的第二步），在该语句正上方单独一行写 `-- atlas:nolint destructive`，再运行 `make migrate-hash`。不要用它掩盖未审查的删除。
4. 运行 `make migrate-check`：确认 schema 与迁移目录一致（再生成一次不会产生新文件），并校验 `atlas.sum`。
5. 迁移文件与 schema 变更在同一提交中提交。

规则：

- 已提交的迁移文件不再修改，修正也要新增迁移文件。
- 删列、改名按 expand-contract 分两次发布：第一次发布新增列并回填，第二次发布在确认没有代码使用旧列后再删除。
- 迁移只有向前（up），没有 down。回滚依靠升级前的备份。

## 3. 上游同步流程

1. 合并上游。
2. 若上游改动了 `internal/ent/schema/` 或 `schemahook`：运行 `make generate`，再运行 `make migrate-diff name=upstream_<version>`。若 Ent 删除了列或表，lint 会失败；确认删除符合预期后，在该语句上方加 `-- atlas:nolint destructive`（见第 2 节）。
3. 检查上游对 `internal/server/db/ent.go` 中改表前清理逻辑（如 `cleanupRemovedProviderQuotaTypes`）的改动。`versioned` 模式不执行这些清理，要把它们改写成 SQL，放在新迁移文件中 DDL 之前。
4. 审查生成的 SQL，重点看 `DROP`、类型变更、NOT NULL 新列。任何手工修改之后都要运行 `make migrate-hash`。
5. 提交，等 CI 通过后再发布。

注意：Ent 的 diff 不会执行通过 `schema.WithHooks` 注册的 Creator 钩子（如 `schemahook.V0_3_0`）。只要没有 `schemahook` 改变最终结构，这就不影响结果，由 CI 的结构等价测试把关。上游新增或修改 `schemahook` 时，务必查看该测试的结果。

## 4. 发布与升级

以下命令都在服务器的 `deploy/iqlater` 目录（compose 项目所在目录）执行。

1. 先做备份（见 `deploy/iqlater/scripts/backup.sh`）。Atlas 的版本记录表 `atlas_schema_revisions` 位于 `public` schema，会随 `pg_dump` 一起备份，不要删除它。
2. 修改 `.env` 中的 `IQLATER_VERSION`。axonhub 与 `migrate` 镜像使用相同的标签，保证迁移与代码版本一致。
3. 拉取镜像：

   ```bash
   docker compose pull
   ```

4. 预览迁移：

   ```bash
   docker compose run --rm migrate migrate status --env deploy
   docker compose run --rm migrate migrate apply --env deploy --dry-run
   ```

5. 启动：

   ```bash
   docker compose up -d
   ```

   `migrate` 服务先执行 `migrate apply --env deploy`，成功后 axonhub 才会启动；迁移失败（包括 `atlas.sum` 校验失败）时 axonhub 不启动。

说明：

- `migrate` 服务以用户 `65532:65532` 运行。`atlas` 二进制权限为 `0111`，root 在 `cap_drop: ALL` 后无法执行它。
- 连接串通过环境变量 `DATABASE_URL` 传入，`atlas.hcl` 中的 `deploy` 环境读取它，迁移目录为镜像内的 `/migrations`。

## 5. 失败处理

- **迁移失败或升级后异常**：从升级前的备份恢复数据库，并回退 `.env` 中的 `IQLATER_VERSION`。没有自动回滚。
- **`connected database is not clean`**：Atlas 拒绝对非空且没有版本记录的库执行迁移。如果部署库曾在 `auto` 模式下建过表（例如本机演练过 compose，或服务器试启动过），首次切换到 `versioned` 前要删除 `postgres_data` 卷重建空库：

  ```bash
  docker compose down
  docker volume rm iqlater_postgres_data
  ```

  只有在库中没有需要保留的数据时才可以这样做（例如生产首次上线之前）。有生产数据的库绝不能这样做：本方案不提供 baseline 标记流程。卷名以 `docker volume ls` 的输出为准。

## 6. POSTGRES_PASSWORD 使用 hex 字符

`POSTGRES_PASSWORD` 会被直接拼进 `migrate` 和 axonhub 的连接 URL（`postgres://iqlater:<密码>@postgres:5432/...`）。密码中的 `@`、`:`、`/`、`?`、`#`、`%` 等字符不做 URL 转义会破坏连接串。因此用 hex 字符生成：

```bash
openssl rand -hex 24
```

## 7. Atlas 版本与社区版功能

- 迁移镜像固定为 `arigaio/atlas:1.4.0-community`（开源构建），见 `deploy/iqlater/migrate/Dockerfile`，CI 使用同一个标签。`latest-community` 是 canary 构建，不使用。升级版本时同步修改 Dockerfile 和 `.github/workflows/iqlater-migrations.yml`。
- 该社区版镜像的 `migrate lint` 无需登录即可使用，CI 用它检查本次变更新增的迁移文件。

## 8. CI

`.github/workflows/iqlater-migrations.yml` 在指向 `main` 的 pull request 和 push 到 `main` 时运行（带 PostgreSQL 18 service），依次执行：

1. `make migrate-check`：schema 已体现在迁移中，`atlas.sum` 有效。
2. `TestVersionedMigrationsMatchAutoMigrate`：在空库上执行全部版本化迁移，与 `auto` 模式 `Schema.Create` 建出的结构比较，差异必须为空。
3. `atlas migrate lint --latest <n>`：检查本次变更新增的迁移文件（pull request 与 `main` 的 merge base 比较，push 与推送前的提交比较；`n` 为新增 `.sql` 文件数，没有新增则跳过）。Atlas 镜像不含 `git`，所以不使用 `--git-base`。破坏性变更（`DROP COLUMN`、`DROP TABLE`）会使作业失败，除非该语句带 `-- atlas:nolint destructive`（见第 2 节）。

