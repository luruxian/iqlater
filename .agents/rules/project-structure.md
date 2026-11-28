---
alwaysApply: true
---

# Project Structure Rules

新增文件、包、目录或功能前，先按本规则确定落点。规则的核心是三件事：依赖只能单向向下，按业务领域而不是技术类型组织代码，新代码沿用所在目录的既有模板。

## 顶层目录

| 目录 | 职责 |
|------|------|
| `cmd/` | 可执行入口：`axonhub`（服务）、`axonhub-desktop`（独立 Go 模块）、`pack-plugin`、`schema`。只做参数解析与 FX 组装，不写业务逻辑 |
| `conf/` | 配置结构与加载 |
| `internal/` | 主模块私有代码，见下文分层 |
| `llm/` | 独立 Go 模块：LLM 协议模型、transformer、pipeline、httpclient。不得 import 主模块任何包 |
| `frontend/` | 管理后台 SPA |
| `plugins/` | 插件示例、社区插件与 registry |
| `integration_test/` | 按上游协议（openai / anthropic / gemini）组织的端到端集成测试 |
| `scripts/` | 开发、发布、迁移脚本 |
| `deploy/`、`dev/`、`docker-compose.yml`、`Dockerfile` | 部署与本地开发环境 |
| `docs/` | 用户与开发文档（`zh/` 与 `en/` 结构保持对称）；长期有效的开发约定与架构说明放 `docs/<lang>/development/`。 |
| `.agents/` | AI 协作规则（`rules/`）、技能（`skills/`）、总结（`summary/`） |

1. 不新增顶层目录。确有需要时先说明理由，并同步更新本表。
2. 不在仓库根目录新增笔记、临时脚本或产物，临时文件放已被忽略的 `tmp/`。
3. 本仓库是 AxonHub 的 fork。来自上游的文件（包括根目录已有的文档）保持原位置，不为整理结构而移动或重命名，以免同步上游时产生冲突。

## 后端分层

依赖方向自上而下，下层不得 import 上层，同层之间不得形成环：

```
cmd/axonhub
  └─ internal/server              组装：server.go、routes.go、FX 模块、生命周期
       ├─ api/  gql/  middleware/   传输层：REST handler、GraphQL resolver、Gin 中间件
       ├─ orchestrator/  plugin/    LLM 请求编排、插件运行时
       ├─ backup/ gc/ video_storage/ scheduler/ events/ safeguards/ ...  后台与支撑子系统
       └─ biz/                      领域服务（业务规则、事务、缓存）
            └─ internal/ent          数据访问（Ent 生成代码 + schema）
基础层（任何上层都可依赖，自身不依赖 internal/server/**）：
  internal/objects  contexts  authz  scopes  log  tracing  metrics  build  dumper  pkg/x*
外部独立模块：llm/
```

1. 传输层只做协议转换：解析参数、鉴权边界、调用 `biz` 或 `orchestrator`、组装响应。业务规则、跨表写入和缓存逻辑放 `biz`。
2. `biz` 不得 import `api`、`gql`、`middleware`、`orchestrator`、`plugin`。`biz` 需要回调上层能力时，在 `biz` 定义接口，由上层实现并通过 FX 注入。
3. `internal/ent/schema` 只依赖 `objects`、`scopes`、`pkg` 等基础包，不依赖 `internal/server/**`。
4. `internal/pkg/x*` 是与业务无关的通用工具，只能依赖标准库、第三方库、其他 `pkg` 和 `log`，不得依赖 `internal/server/**` 和 `internal/ent`。不要新建 `util`、`common`、`helpers` 这类无领域含义的包。
5. `internal/objects` 放被 Ent JSON 字段、GraphQL 和 biz 共同使用的值对象，不放带依赖的服务逻辑。
6. 已知的历史反向依赖不作为先例：`biz` → `gql/qb`、`internal/ent/migrate/datamigrate` → `biz`。不要新增同类依赖；涉及这些位置的改动，优先把被依赖的部分下沉到基础层。

## 后端新增代码落点

| 要新增的东西 | 位置与要求 |
|--------------|------------|
| 新业务领域服务 | `internal/server/biz/<domain>.go`，构造函数 `New<Domain>Service`，在 `biz/fx_module.go` 注册 |
| 已有领域的子能力 | 同包新文件 `biz/<domain>_<topic>.go`，前缀与领域一致（如 `channel_rate_limit.go`），不要塞进已过千行的主文件 |
| 自包含、无需 biz 内部状态的大块逻辑 | `biz/<domain>/` 子包（参考 `biz/provider_quota/`），子包不得 import `biz` |
| GraphQL 类型与操作 | `gql/<domain>.graphql` → `make generate` → `gql/<domain>.resolvers.go` 只改方法体；辅助函数放 `gql/<domain>_helpers.go` |
| REST 接口 | `internal/server/api/<domain>.go`，在 `internal/server/routes.go` 注册路由 |
| HTTP 中间件 | `internal/server/middleware/` |
| 数据表 | `internal/ent/schema/<entity>.go`，遵循 [ent-graphql.md](ent-graphql.md)、[database-indexes.md](database-indexes.md) |
| 数据迁移（非 schema 变更） | `internal/ent/migrate/datamigrate/`，按版本命名 |
| 定时 / 后台任务 | 使用 `internal/server/scheduler`，任务逻辑放所属领域的 `biz` 或子系统 |
| 请求上下文取值 | `internal/contexts`，不要在业务包里自定义 ctx key |
| 权限范围与规则 | `internal/scopes`（scope 定义与 rule）、`internal/authz`（鉴权执行） |
| 新 LLM 渠道 / 协议 | `llm/transformer/<provider>/`，按 [workflows/add-channel.md](workflows/add-channel.md) 执行 |
| 通用工具 | 先找 `lo` 和已有 `internal/pkg/x*`；确属通用再新建 `internal/pkg/x<name>/` |

## 后端文件与包约定

1. 文件名使用 lower snake case；测试与被测文件同目录同前缀（`channel_probe.go` / `channel_probe_test.go`）。
2. 包名使用单个小写单词，与目录名一致，不使用下划线。
3. 新文件以约 800 行为上限。修改已超过 1500 行的文件时，新增的独立子能力拆到同前缀新文件，不要继续扩大原文件。
4. 生成代码只能通过 `make generate` 更新，不手工修改；手写逻辑不得放进生成文件。
5. 新服务按既有 FX 模式在所属模块的 `fx_module.go` / `module.go` 中 `fx.Provide`，不使用包级全局变量持有服务实例。

## 前端分层

依赖方向：`routes` → `features` → 共享层（`components`、`hooks`、`stores`、`lib`、`utils`、`gql`、`config`、`context`、`locales`）。

1. `routes/` 只做路由声明与守卫：`createFileRoute` + `RouteGuard` + 引入 `@/features/<feature>` 的默认导出。不在路由文件写页面逻辑。项目级页面放 `routes/_authenticated/project/`，管理级页面放 `routes/_authenticated/<feature>/`。
2. 共享层不得 import `@/features/**`。共享组件需要业务数据时，由调用方通过 props 传入，或把对应的数据 hook 下沉到 `hooks/` / `gql/`。
3. feature 之间尽量不互相 import。确需复用时，只从对方的 `index.ts` 或 `data/` 的公开导出引入，不深入对方 `components/` 内部；被三个以上 feature 复用的内容应提升到共享层。
4. `components/ui/` 是 shadcn 生成的基础组件，只通过 shadcn CLI 添加，不放业务组件；业务通用组件放 `components/` 根下或 `components/<group>/`。
5. 已知的历史反向依赖不作为先例：`components/` 下的 guard、layout 和若干业务组件 import 了 `@/features/**`。不要新增同类依赖。

## 前端 feature 模板

新增 feature 放 `frontend/src/features/<feature>/`，目录名使用 kebab-case，与路由路径一致：

```
features/<feature>/
  index.tsx              页面入口，默认导出页面组件
  components/            本 feature 的组件，文件名以 feature 名为前缀（users-table.tsx、users-action-dialog.tsx）
  context/<feature>-context.tsx   对话框开关、当前行等页面级 UI 状态
  data/
    schema.ts            zod schema 与由其推导的类型
    <feature>.ts         GraphQL 文档与 React Query hooks（useXxx / useCreateXxx ...）
  hooks/                 可选，本 feature 专用 hook
  utils/                 可选，本 feature 专用纯函数
```

1. 只在本 feature 使用的 GraphQL 文档放 `data/<feature>.ts`；多个 feature 共用的文档（如 `me`、roles、scopes）放 `src/gql/<domain>.ts`。
2. 新页面需要导航时，在 `config/nav-items.ts` 添加导航项，并在 `config/route-permission.ts` 配置所需 scope。
3. 文件名使用 kebab-case；hook 文件使用 `use-<name>.ts`（已有的 camelCase hook 文件保持原名，不顺手改名）。
4. 已存在的目录名拼写问题（如 `proejct-users`）不要顺手重命名，重命名需单独变更并同步所有引用。
5. 单个组件文件以约 600 行为上限；超出时把子表单、子表格、对话框拆到 `components/` 下的独立文件。

## 新增功能前的检查

1. 先搜索是否已有同领域的服务、文件前缀、feature 或组件，优先扩展已有的，不平行再建一套。
2. 确认新代码只依赖同层或下层；出现需要反向 import 时，改为接口注入或下沉公共部分。
3. 跨前后端的字段变更按 [ent-graphql.md](ent-graphql.md) 的全链路要求一次改完。
