---
alwaysApply: false
globs: "internal/ent/schema/**/*.go, internal/ent/migrate/**/*.go, internal/server/db/**/*.go"
---

# Database Schema Rules

建表与改表规则。索引规则见 [database-indexes.md](database-indexes.md)，Ent 与 GraphQL 代码生成规则见 [ent-graphql.md](ent-graphql.md)。

## 迁移机制（先理解再改表）

启动时由 `internal/server/db/ent.go` 执行 `client.Schema.Create`，关键选项决定了改表的风险：

| 选项 | 含义 | 对改表的影响 |
|------|------|--------------|
| `WithForeignKeys(false)` | 不创建数据库外键 | `entsql.OnDelete(...)` 不会生成数据库约束，也不会级联删除。关联数据的清理必须在代码中完成 |
| `WithDropColumn(true)` | 删除 schema 中已不存在的列 | 删字段或改字段名会直接丢弃该列数据，不可恢复 |
| `WithDropIndex(true)` | 删除 schema 中已不存在的索引 | 改索引名等于删除后重建 |
| `filterEquivalentDefaultChanges()` | 过滤仅格式不同的默认值差异 | 避免 SQLite 每次启动重建表；新增时间默认值沿用 `entsql.DefaultExpr("CURRENT_TIMESTAMP")` |

1. 结构变更只改 `internal/ent/schema/`，运行 `make generate`，由自动迁移生效。不新增手写 SQL 迁移文件；`internal/ent/migrate/migrations/` 下的 SQL 是历史遗留，不作为先例。
2. 历史数据需要补齐或修复时，在 `internal/ent/migrate/datamigrate/` 新增按版本命名的迁移（`v<version>.go` + 测试），并在 `migrator.go` 中按版本顺序注册。数据迁移必须幂等，见该目录 `README.md`。
3. 必须在 DDL 之前清理的数据（例如删除枚举值前清掉使用该值的行），放在 `db/ent.go` 中 `Schema.Create` 之前执行，参考 `cleanupRemovedProviderQuotaTypes`。
4. 所有变更必须同时适用于 SQLite、PostgreSQL、MySQL/TiDB。
5. iqlater 生产使用 `versioned` 模式（`AXONHUB_DB_MIGRATION_MODE=versioned`）：启动时不执行清理和 `Schema.Create`，表结构由 `internal/ent/migrate/versioned/postgres/` 下的 Atlas 迁移文件决定。本地开发库与 SQLite 仍用 `auto` 模式。
6. 改 schema 后，除 `make generate` 外，必须在同一提交中带上 `make migrate-diff name=<name>` 生成的迁移文件和更新后的 `atlas.sum`，并人工审查生成的 SQL（重点看 `DROP`、类型变更、NOT NULL 新列）。
7. 已提交的迁移文件不再修改，修正也要新增迁移文件，否则 `atlas.sum` 校验失败或已部署环境与仓库不一致。
8. 删列、改名按 expand-contract 分两次发布：先发布新增列和双写或回填，确认无代码再使用旧列后，下一次发布再删除旧列。
9. 第 3 条的改表前清理逻辑在 `versioned` 模式下不会执行。上游新增或修改这类清理时，要改写成 SQL，放在迁移文件中 DDL 之前。

## 表与文件命名

1. 一个实体一个文件：`internal/ent/schema/<entity>.go`，文件名为 lower snake case 单数，类型名为 PascalCase 单数（`prompt_protection_rule.go` / `PromptProtectionRule`）。
2. 表名使用 Ent 默认推导（snake case 复数，如 `prompt_protection_rules`），不使用 `entsql.Table` 自定义表名。
3. 字段名使用 lower snake case。布尔字段使用 `is_` / `has_` / `enable_` 等可读前缀或形容词，不使用否定命名（如 `not_deleted`）。
4. 表名、字段名一经发布不再修改。确需在代码中改名时，用 `StorageKey("<old_name>")` 保留原列名，不要让迁移删列重建。

## Mixin 选择

| 实体类型 | Mixin | 示例 |
|----------|-------|------|
| 可被用户管理的业务实体 | `TimeMixin{}` + `schematype.SoftDeleteMixin{}` | `Prompt`、`Channel`、`APIKey`、`Project` |
| 追加写入的日志、记录类高频表 | 仅 `TimeMixin{}`，过期数据由 `internal/server/gc` 物理清理 | `Request`、`UsageLog`、`Trace`、`Thread` |
| 关联表、内部存储表 | 可不使用 Mixin，自行声明需要的时间字段 | `UserRole`、`RequestBodyChunk`、`ChannelProbe` |

1. 新的业务实体默认同时使用 `TimeMixin` 与 `SoftDeleteMixin`。不使用时在 schema 类型注释中写明原因。
2. 不在业务表中手写 `created_at` / `updated_at`，统一由 `TimeMixin` 提供（UTC、`created_at` 不可变、GraphQL 输入中跳过）。

## 软删除

1. 软删除字段为 `deleted_at int`：未删除为 `0`，已删除为 Unix 秒。不要改为可空时间字段，否则 NULL 在唯一索引中各方言语义不一致。
2. 软删除表上的业务唯一约束必须把 `deleted_at` 放在唯一索引末尾（如 `project_id, name, deleted_at`），保证删除后可以重名创建。
3. 查询默认自动过滤已删除行。需要读取已删除行或物理删除时，使用 `schematype.SkipSoftDelete(ctx)`，并限定在 GC、数据迁移等内部流程中。

## 主键与关联

1. 主键使用 Ent 默认的自增 `int` `id`，不自定义主键。对外通过 GraphQL GUID 暴露，前端用 `extractNumberID` 还原。
2. 引用本库实体的外键字段命名为 `<entity>_id`，类型 `field.Int`，并声明对应 edge 绑定该字段：

   ```go
   field.Int("project_id").Immutable(),
   // ...
   edge.From("project", Project.Type).Ref("prompts").Field("project_id").Required().Unique().Immutable(),
   ```

3. 字段与 edge 的 `Required` / `Optional`、`Immutable` 必须一致。
4. 外部系统或协议中的字符串标识（如 `model_id`、`trace_id`、`external_id`、`plugin_id`）不是外键，用 `field.String` 并设置合理的 `MaxLen`。
5. 因为未启用数据库外键，删除父实体时由代码负责关联数据：
   - 软删除实体：通常保留子数据，由查询条件和权限保证不可见。
   - 物理删除或 GC：在 `RunInTransaction` 中显式删除子表，或由 `internal/server/gc` 按顺序清理（参考 `gc.go`、`gc_bodies.go`）。
   - `entsql.OnDelete(...)` 仅作为关系意图的声明保留，不能替代上述代码。

## 租户与归属

1. 属于项目的数据必须有 `project_id`：`field.Int("project_id").Immutable()`，在 GraphQL create/update 输入中 `entgql.Skip`，由服务端从上下文写入，不接受客户端传入。
2. 属于个人的数据使用 `user_id`，同样不可由客户端指定归属。
3. 租户字段必须进入相关索引的前缀，并在查询中保留租户条件，见 [database-indexes.md](database-indexes.md)。

## 字段类型

| 场景 | 写法 |
|------|------|
| 有界短字符串（名称、标识、哈希） | `field.String` + `MaxLen(n)`；被索引的字符串必须设置 `MaxLen` |
| 长文本（prompt、模板、描述性大段内容） | `field.String` + `SchemaType(map[string]string{dialect.MySQL: "mediumtext"})`，超大内容用 `longtext`。MySQL 下 `field.String` 默认是 `varchar(255)` |
| 二进制 | `field.Bytes` + MySQL `mediumblob` / `longblob` |
| 状态、类型等有限取值 | `field.Enum(...).Values(...)`，取值为 lower snake case，必须给 `Default` |
| 字符串列表 | `field.Strings` |
| 结构化配置 | `field.JSON("<name>", objects.<Type>{})`，类型定义在 `internal/objects` |
| 时间点 | `field.Time`，可选的时间用 `Optional().Nillable()` |
| 价格配置 | 放在 `internal/objects` 的 JSON 结构中，使用 `shopspring/decimal`（参考 `objects/price.go`），避免浮点误差 |
| 统计用的成本汇总 | `field.Float`，可选时用 `Optional().Nillable()`（参考 `UsageLog.total_cost`），不用于计费结算 |

1. 枚举新增取值只追加在末尾。删除取值前，必须先在 DDL 之前清理存量数据（见“迁移机制”第 3 条）；在 MySQL/TiDB 下，删除枚举值会重写列。
2. JSON 字段的 Go 结构体是持久化格式：只新增带 `omitempty` 的字段，不改已有 JSON tag，不改变已有字段的类型与语义，以兼容历史数据。规则与 [cache-compat.md](cache-compat.md) 一致。
3. 不使用 `map[string]any` 作为 JSON 字段类型，除非内容确实是自由格式（如插件数据）。

## 空值、默认值与不可变

1. 给已有表新增列时，必须提供 `Default(...)`，或声明 `Optional()`。否则在已有数据的表上，PostgreSQL/MySQL 添加 NOT NULL 列会失败。
2. 只有当 NULL 与零值语义不同（“未设置”不同于 `0` / `""`）时才使用 `Nillable()`；其余情况用非空字段加默认值。
3. 创建后不应改变的字段（归属、来源、外部标识、记录类表的数据字段）使用 `Immutable()`。

## 安全与权限

1. 凭证、密码、token 及其哈希使用 `Sensitive()`，并在 GraphQL 中 `entgql.Skip` 输出，参考 `Channel.credentials`、`User.password`、`Invitation.token_hash`。
2. 每个实体必须定义 `Policy()`：
   - 业务实体使用 `scopes.Policy`，组合 `OwnerRule`、项目级和系统级读写 rule。
   - 仅供内部使用的存储表显式使用 `privacy.AlwaysDenyRule()`，由内部流程通过受控 bypass 访问，参考 `RequestBodyChunk`。
3. 不定义 `Policy()` 等于放行所有访问。新实体不得省略；省略时必须在类型注释中写明原因。权限模型见 `docs/zh/development/ent-privacy-governance-plan.md`。

## GraphQL 暴露

1. 需要在后台展示的实体添加 `entgql.QueryField()`、`entgql.RelayConnection()`；需要通用增改时添加 `entgql.Mutations(entgql.MutationCreate(), entgql.MutationUpdate())`。
2. 服务端维护的字段（归属、统计、内部状态）在 create/update 输入中 `entgql.Skip`。
3. 可排序字段添加 `entgql.OrderField("<UPPER_SNAKE>")`，并确保有对应索引。

## 注释

1. 每个 schema 类型有一行英文类型注释。含义不直观的字段使用英文 `Comment(...)`，写明单位、取值含义和来源，例如 `Comment("prompt insertion order, smaller values are inserted first")`。

## 变更检查清单

1. 已运行 `make generate`，并检查 `internal/ent/migrate/schema.go` 中的列类型、可空性、默认值和索引符合预期。
2. 没有删字段或改字段名。确需删除时，已确认数据可丢弃，并在变更说明中写明。
3. 新列可以在已有数据的库上添加成功（有默认值或可空）。
4. 已在 SQLite 下跑相关测试；涉及类型、长度或索引时，确认 PostgreSQL 与 MySQL 也可用。
5. 需要补历史数据时，已添加幂等的 datamigrate 及测试。
6. GraphQL 与前端的读写链路已按 [ent-graphql.md](ent-graphql.md) 同步。
7. `make migrate-check` 通过，迁移文件已随 schema 变更在同一提交中提交。
