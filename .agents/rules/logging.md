---
alwaysApply: false
globs: "**/*.go"
---

# Logging Rules

## 日志入口

1. 主模块与 `cmd/axonhub-desktop/` 统一使用 `github.com/looplj/axonhub/internal/log`。不要在业务代码中直接 import `go.uber.org/zap` 或 `log/slog` 打日志；字段构造也用 `log.String`、`log.Int`、`log.Cause` 等包装函数（`log.Field` 即 `zap.Field`）。
2. `llm/` 是独立模块，不能依赖 `internal/log`，使用 `log/slog` 的 `*Context` 变体（`slog.DebugContext`、`slog.WarnContext` 等）。主模块启动时通过 `slog.SetDefault(log.GetGlobalLogger().AsSlog())` 把 slog 路由到 zap，并通过 `logger.AsSlog()` 注入需要 `*slog.Logger` 的组件。不要使用不带 ctx 的 `slog.Info` / `slog.Error`，否则会丢失链路字段。
3. 默认使用包级函数 `log.Debug/Info/Warn/Error(ctx, msg, fields...)`。只有需要按名称过滤（`log.includes` / `log.excludes` 配置）的独立子系统才用 `log.Get(name)` 创建命名 logger。
4. 业务代码不使用 `log.Panic`。启动期不可恢复的错误应返回 error，由 FX 生命周期处理。

## Context

1. 第一个参数必须传当前调用链的 `ctx`。请求链路中的 ctx 会由 `internal/tracing` 的 hook 自动附加 `trace_id`、`request_id`、`operation_name`，ctx 已取消或带 deadline 时还会附加 `context_error`、`context_deadline`。
2. 不要手动重复添加 `trace_id`、`request_id`、`operation_name`。
3. `context.Background()` 只用于启动、关闭、定时任务等确实没有请求上下文的场景。从请求派生的异步任务应基于请求 ctx 派生（必要时用 `context.WithoutCancel`），以保留链路字段。

## 日志级别

| 级别 | 用途 | 示例 |
|------|------|------|
| `Debug` | 排查细节、每次请求内部的中间步骤、热路径事件 | 选中候选渠道、重试同一渠道、限流计数 |
| `Info` | 低频的生命周期与状态变化 | 配置加载、后台任务完成、渠道被自动禁用、备份完成 |
| `Warn` | 可恢复的失败：已降级、重试、跳过，或外部依赖异常但主流程继续 | 缓存失效失败、Webhook 通知失败、模型列表拉取失败 |
| `Error` | 操作失败且没有返回给调用方处理，需要人关注；goroutine panic | 后台任务失败、数据写入失败被吞掉、panic recover |

1. 每次 LLM 请求都会经过的主链路（pipeline、orchestrator、transformer、middleware）不新增 `Info`，使用 `Debug`。
2. 循环内不要逐条打 `Info` / `Warn`，汇总后打一条，并带上 `count` 等统计字段。
3. 构造代价高的字段（序列化大对象、拼接请求体等）用 `log.DebugEnabled(ctx)` 等判断包裹，避免在日志关闭时也付出计算成本。

## 错误记录

1. 错误字段统一使用 `log.Cause(err)`（输出键为 `error`）。不要使用 `zap.Error(err)` 或 `log.Any("error", err)`。
2. 同一条日志需要多个错误时，额外的错误用 `log.NamedError("<purpose>_error", err)`。
3. 记录或返回，二选一：已经 `return` 给调用方的错误，本层不再打 `Error`，由最终处理它的边界（resolver、handler、后台任务、goroutine）记录一次。被吞掉、降级或忽略的错误必须在吞掉的位置记录。
4. 手动启动的 goroutine 的 `recover` 日志使用 `Error` 级别，消息以 `panicked` 结尾，并带上 `log.Any("panic", r)` 和 `log.Stack("stack")`。

## 消息格式

1. 消息使用英文，小写开头，不带结尾标点，描述发生了什么。失败统一写成 `failed to <verb> <object>`，例如 `failed to sync models for channel`。
2. 消息中不要拼接变量（不要用 `fmt.Sprintf`），变量全部放进字段，保证同类日志可以按消息聚合检索。
3. 不使用 `Successfully ...`、`Running ...` 这类冗余前缀，直接写结果，例如 `cleaned up response bodies`。

## 字段命名

1. 字段键使用 lower snake case，例如 `channel_id`。不要使用 camelCase。
2. 实体标识使用 `<entity>_id`，值用 `log.Int` / `log.Int64` 记录数值 ID，不要记录 GUID 字符串；需要可读性时再加 `<entity>_name`。
3. 优先复用已有键名：

   | 键 | 含义 |
   |----|------|
   | `channel_id` / `channel_name` | 渠道 |
   | `model` | 请求或上游使用的模型名 |
   | `model_id` | 模型实体 ID |
   | `project_id` / `user_id` / `api_key_id` | 租户与身份 |
   | `plugin_id` / `installation_id` | 插件 |
   | `usage_log_id` / `request_execution_id` / `execution_id` | 请求记录 |
   | `provider` / `api_format` | 上游类型与协议格式 |
   | `status` / `error_code` / `reason` | 结果与原因 |
   | `count` / `duration` | 统计与耗时（耗时用 `log.Duration`） |
   | `panic` | recover 得到的值 |

4. 以下键由编码器或 hook 占用，业务代码不要使用：`time`、`level`、`label`、`logger`、`message`、`stacktrace`、`trace_id`、`request_id`、`operation_name`、`context_error`、`context_deadline`。

## 敏感信息

1. 不得记录明文 API Key、渠道凭证、OAuth client secret / access token / refresh token、密码、会话 token、系统 `SecretKey`。API Key 需要定位时使用 `xsecret.MaskAPIKey(key)`，或只记录 `api_key_id`。
2. 不得记录 `Authorization`、`Cookie`、`X-Api-Key` 等认证相关请求头。需要记录请求头时先过滤这些头。
3. 用户请求体和模型响应体（prompt、completion）属于用户数据，默认不记录。排查需要时只在 `Debug` 级别记录，优先记录长度、哈希或截断片段，而不是完整内容。
4. 不要用 `log.Any` 直接输出 Ent 实体或包含凭证的配置结构体（例如 `Channel`、`System`、`conf.Config`），只挑需要的字段单独记录。
