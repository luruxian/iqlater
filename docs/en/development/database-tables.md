# AxonHub Database Tables

This document lists the columns and relationships of all 33 tables, grouped by data domain, as Mermaid ER diagrams. If you only need how entities relate, see the [Entity Relationship Diagram (ERD)](erd.md).

It is exported from the Ent target schema in `internal/ent/migrate` (`migrate.Tables`), which is the definition Atlas diffs against when generating versioned migrations. It is not updated automatically when the schema changes; `internal/ent/schema/` is the source of truth.

## How to read

- A solid line (`--`) is a database foreign key. A dashed line (`..`) is a logical reference: the column stores another table's ID, but there is no foreign key constraint.
- `o|` means optional (zero or one), `||` means required (exactly one), `o{` means zero or more.
- Column markers: `PK` is the primary key, `FK` is a foreign key column, `UK` is a unique column. In comments, `nullable` means the column may be null, `enum:` lists the allowed values, `default:` gives the default, and `logical ref` marks a logical reference column.
- Types come from Ent: `time` is a timestamp, `json` is a JSON column, and `enum` is an enum stored as a string.
- Most business tables have `created_at` and `updated_at`; soft-deleted tables also have `deleted_at` (0 means not deleted).
- Each domain diagram shows only references from its own tables; referenced tables from other domains appear by name only, without columns.

## Overview

Table-to-table relationships only, without columns.

```mermaid
erDiagram
    projects ||--o{ api_key_profile_templates : project_id
    projects ||--o{ api_keys : project_id
    users o|--o{ api_keys : user_id
    channel_model_prices ||--o{ channel_model_price_versions : channel_model_price_id
    channels ||--o{ channel_model_prices : channel_id
    users o|--o{ channel_override_templates : user_id
    channels ||--o{ channel_probes : channel_id
    projects ||--o{ invitations : project_id
    users ||--o{ oidc_identities : user_id
    projects ||--o{ prompts : project_id
    channels ||--o| provider_quota_status : channel_id
    requests o|--o{ request_body_chunk_refs : request_id
    request_body_chunks ||--o{ request_body_chunk_refs : chunk_id
    request_executions o|--o{ request_body_chunk_refs : execution_id
    channels o|--o{ request_executions : channel_id
    data_storages o|--o{ request_executions : data_storage_id
    requests ||--o{ request_executions : request_id
    api_keys o|--o{ requests : api_key_id
    channels o|--o{ requests : channel_id
    data_storages o|--o{ requests : data_storage_id
    projects ||--o{ requests : project_id
    traces o|--o{ requests : trace_id
    projects o|--o{ roles : project_id
    projects ||--o{ threads : project_id
    projects ||--o{ traces : project_id
    threads o|--o{ traces : thread_id
    channels o|--o{ usage_logs : channel_id
    projects ||--o{ usage_logs : project_id
    requests ||--o{ usage_logs : request_id
    users ||--o{ user_projects : user_id
    projects ||--o{ user_projects : project_id
    users ||--o{ user_roles : user_id
    roles ||--o{ user_roles : role_id
    users o|..o{ requests : user_id
    users o|..o{ usage_logs : user_id
    api_keys o|..o{ usage_logs : api_key_id
    projects ||..o{ request_executions : project_id
    projects ||..o{ request_body_chunks : project_id
    roles o|..o{ invitations : role_id
    channels ||..o{ channel_model_price_versions : channel_id
    plugins ||..o{ plugin_schedules : installation_id
    plugins ||..o{ plugin_storages : installation_id
    plugin_schedules ||..o{ plugin_schedule_runs : schedule_id
    plugin_table_storages ||..o{ plugin_table_operations : storage_id
    plugins ||..o{ plugin_table_storages : plugin_id
```

## Identity and access

Users, projects, memberships, roles, API keys, login identities, and invitations.

```mermaid
erDiagram
    users {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string email
        enum status "enum: activated/deactivated; default: activated"
        string prefer_language "default: en"
        string password
        string first_name "default: ''"
        string last_name "default: ''"
        string avatar "nullable"
        bool is_owner "default: false"
        json scopes "nullable"
    }
    projects {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description "default: ''"
        enum status "enum: active/archived; default: active"
        json profiles "nullable"
    }
    user_projects {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        bool is_owner "default: false"
        json scopes "nullable"
        int user_id FK
        int project_id FK
    }
    roles {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        enum level "enum: system/project; default: system"
        json scopes "nullable"
        int project_id FK "nullable; default: 0"
    }
    user_roles {
        int id PK
        time created_at "nullable; default: CURRENT_TIMESTAMP"
        time updated_at "nullable; default: CURRENT_TIMESTAMP"
        int user_id FK
        int role_id FK
    }
    api_keys {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string key
        string name
        enum type "enum: user/service_account/noauth/personal; default: user"
        enum status "enum: enabled/disabled/archived; default: enabled"
        json scopes "nullable"
        json profiles "nullable"
        json allowed_ips "nullable"
        int project_id FK "default: 1"
        int user_id FK "nullable"
    }
    api_key_profile_templates {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description "default: ''"
        json profile "nullable"
        bool apply_to_new_personal_keys "default: false"
        int project_id FK
    }
    oidc_identities {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string issuer
        string subject
        string email "nullable"
        string idp_name "nullable"
        time last_login_at "nullable"
        int user_id FK
    }
    invitations {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string token_hash
        int role_id "nullable; logical ref"
        time expires_at "nullable"
        int max_uses "default: 1"
        int used_count "default: 0"
        int project_id FK
    }
    projects ||--o{ api_key_profile_templates : project_id
    projects ||--o{ api_keys : project_id
    users o|--o{ api_keys : user_id
    projects ||--o{ invitations : project_id
    users ||--o{ oidc_identities : user_id
    projects o|--o{ roles : project_id
    users ||--o{ user_projects : user_id
    projects ||--o{ user_projects : project_id
    users ||--o{ user_roles : user_id
    roles ||--o{ user_roles : role_id
    roles o|..o{ invitations : role_id
```

## Channels and models

Upstream provider channels, model definitions, channel pricing and its history, channel probes, and quota status.

```mermaid
erDiagram
    channels {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        enum type "enum: openai/openai_responses/atlascloud/cline/codex/vercel/anthropic/anthropic_aws/anthropic_gcp/gemini_openai/gemini/gemini_vertex/deepseek/deepseek_anthropic/deepinfra/qiniu/fireworks/doubao/doubao_anthropic/moonshot/moonshot_anthropic/zhipu/zai/zhipu_anthropic/zai_anthropic/anthropic_fake/openai_fake/openrouter/xiaomi/xiaomi_anthropic/xai/xai_responses/xai_subscription/ppio/siliconflow/volcengine/volcengine_anthropic/longcat/longcat_anthropic/minimax/minimax_anthropic/aihubmix/aihubmix_anthropic/burncloud/modelscope/bailian/bailian_responses/bailian_anthropic/moonshot_coding/jina/github/github_copilot/claudecode/cerebras/antigravity/nanogpt/nanogpt_responses/opencode_go/opencode_go_anthropic/ollama/ollama_anthropic/evolink/evolink_anthropic/groq/qiniu_anthropic/fenno/zenmux/zenmux_responses/zenmux_anthropic/zenmux_gemini/zenmux_video/commandcode/commandcode_anthropic/typesafe/cloudflare"
        string base_url "nullable"
        string name
        enum status "enum: enabled/disabled/archived; default: disabled"
        json credentials
        json disabled_api_keys "nullable"
        json supported_models
        json manual_models "nullable"
        bool auto_sync_supported_models "default: false"
        string auto_sync_model_pattern "nullable; default: ''"
        json tags "nullable"
        string default_test_model
        json policies "nullable"
        json settings "nullable"
        int ordering_weight "default: 0"
        string error_message "nullable"
        time auto_disabled_at "nullable"
        time auto_disable_expires_at "nullable"
        string remark "nullable"
        json endpoints "nullable"
    }
    models {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string developer
        string model_id
        enum type "enum: chat/embedding/rerank/image_generation/video_generation; default: chat"
        string name
        string icon
        string group
        json model_card
        json settings
        enum status "enum: enabled/disabled/archived; default: disabled"
        string remark "nullable"
    }
    channel_model_prices {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string model_id
        json price
        string reference_id UK
        int channel_id FK
    }
    channel_model_price_versions {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int channel_id "logical ref"
        string model_id
        json price
        enum status "enum: active/archived"
        time effective_start_at
        time effective_end_at "nullable"
        string reference_id UK
        int channel_model_price_id FK
    }
    channel_override_templates {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description "nullable"
        string override_parameters
        json override_headers
        json header_override_operations "nullable"
        json body_override_operations "nullable"
        int user_id FK "nullable"
    }
    channel_probes {
        int id PK
        int total_request_count
        int success_request_count
        float64 avg_tokens_per_second "nullable"
        float64 avg_time_to_first_token_ms "nullable"
        int64 timestamp
        int channel_id FK
    }
    provider_quota_status {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        enum provider_type "enum: claudecode/codex/antigravity/xai_subscription/github_copilot/nanogpt/cline/synthetic/neuralwatt/apertis/opencode_go/kimi_code/minimax/zhipu/zai/charm_hyper/zenmux/commandcode/deepseek/ollama"
        enum status "enum: available/warning/exhausted/unknown"
        json quota_data
        time next_reset_at "nullable"
        bool ready "default: true"
        time next_check_at
        string account_key "nullable; default: ''"
        int channel_id FK,UK
    }
    channel_model_prices ||--o{ channel_model_price_versions : channel_model_price_id
    channels ||--o{ channel_model_prices : channel_id
    users o|--o{ channel_override_templates : user_id
    channels ||--o{ channel_probes : channel_id
    channels ||--o| provider_quota_status : channel_id
    channels ||..o{ channel_model_price_versions : channel_id
```

## Requests and usage

Client requests, each upstream execution, deduplicated request body chunks, usage and cost, Thread/Trace grouping, and payload storage.

```mermaid
erDiagram
    requests {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int user_id "nullable; logical ref"
        enum source "enum: api/playground/test; default: api"
        string model_id
        string reasoning_effort "nullable"
        string format "default: openai/chat_completions"
        json request_headers "nullable"
        json request_body
        bool request_body_chunked "default: false"
        json response_headers "nullable"
        json response_body "nullable"
        json response_chunks "nullable"
        string external_id "nullable"
        enum status "enum: pending/processing/completed/failed/canceled"
        bool stream "default: false"
        string client_ip "default: ''"
        string user_agent "default: ''"
        int64 metrics_latency_ms "nullable"
        int64 metrics_first_token_latency_ms "nullable"
        int64 metrics_reasoning_duration_ms "nullable"
        bool content_saved "default: false"
        int content_storage_id "nullable"
        string content_storage_key "nullable"
        time content_saved_at "nullable"
        int api_key_id FK "nullable"
        int channel_id FK "nullable"
        int data_storage_id FK "nullable"
        int project_id FK "default: 1"
        int trace_id FK "nullable"
    }
    request_executions {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int project_id "default: 1; logical ref"
        int channel_api_key_index "nullable"
        string external_id "nullable"
        string model_id
        string upstream_model_id "nullable"
        string format "default: openai/chat_completions"
        string reasoning_effort "nullable"
        string channel_api_key_suffix "nullable"
        json request_body
        bool request_body_chunked "default: false"
        json response_headers "nullable"
        json response_body "nullable"
        json response_chunks "nullable"
        string error_message "nullable"
        int response_status_code "nullable"
        enum status "enum: pending/processing/completed/failed/canceled"
        bool stream "default: false"
        int64 metrics_latency_ms "nullable"
        int64 metrics_first_token_latency_ms "nullable"
        int64 metrics_reasoning_duration_ms "nullable"
        json request_headers "nullable"
        string request_url "nullable"
        bool pass_through_applied "default: false"
        int channel_id FK "nullable"
        int data_storage_id FK "nullable"
        int request_id FK
    }
    request_body_chunks {
        int id PK
        int project_id "logical ref"
        string hash
        bytes data
        time created_at "default: CURRENT_TIMESTAMP"
    }
    request_body_chunk_refs {
        int id PK
        int position
        int request_id FK "nullable"
        int chunk_id FK
        int execution_id FK "nullable"
    }
    usage_logs {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int api_key_id "nullable; logical ref"
        int user_id "nullable; logical ref"
        string model_id
        int64 prompt_tokens "default: 0"
        int64 completion_tokens "default: 0"
        int64 total_tokens "default: 0"
        int64 prompt_audio_tokens "nullable; default: 0"
        int64 prompt_cached_tokens "nullable; default: 0"
        int64 prompt_write_cached_tokens "nullable; default: 0"
        int64 prompt_write_cached_tokens_5m "nullable; default: 0"
        int64 prompt_write_cached_tokens_1h "nullable; default: 0"
        int64 completion_audio_tokens "nullable; default: 0"
        int64 completion_reasoning_tokens "nullable; default: 0"
        int64 completion_accepted_prediction_tokens "nullable; default: 0"
        int64 completion_rejected_prediction_tokens "nullable; default: 0"
        enum source "enum: api/playground/test; default: api"
        string format "default: openai/chat_completions"
        float64 total_cost "nullable"
        json cost_items "nullable"
        string cost_price_reference_id "nullable"
        int channel_id FK "nullable"
        int project_id FK "default: 1"
        int request_id FK
    }
    traces {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        string trace_id UK
        enum status "enum: active/archived/retained; default: active"
        time last_request_at "default: '1970-01-02 00:00:00'"
        int project_id FK
        int thread_id FK "nullable"
    }
    threads {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        string thread_id UK
        enum status "enum: active/archived/retained; default: active"
        int project_id FK
    }
    data_storages {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description
        bool primary "default: false"
        enum type "enum: database/fs/s3/gcs/webdav"
        json settings
        enum status "enum: active/archived; default: active"
    }
    requests o|--o{ request_body_chunk_refs : request_id
    request_body_chunks ||--o{ request_body_chunk_refs : chunk_id
    request_executions o|--o{ request_body_chunk_refs : execution_id
    channels o|--o{ request_executions : channel_id
    data_storages o|--o{ request_executions : data_storage_id
    requests ||--o{ request_executions : request_id
    api_keys o|--o{ requests : api_key_id
    channels o|--o{ requests : channel_id
    data_storages o|--o{ requests : data_storage_id
    projects ||--o{ requests : project_id
    traces o|--o{ requests : trace_id
    projects ||--o{ threads : project_id
    projects ||--o{ traces : project_id
    threads o|--o{ traces : thread_id
    channels o|--o{ usage_logs : channel_id
    projects ||--o{ usage_logs : project_id
    requests ||--o{ usage_logs : request_id
    users o|..o{ requests : user_id
    users o|..o{ usage_logs : user_id
    api_keys o|..o{ usage_logs : api_key_id
    projects ||..o{ request_executions : project_id
    projects ||..o{ request_body_chunks : project_id
```

## Prompts

Project prompts and global prompt protection rules.

```mermaid
erDiagram
    prompts {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description "default: ''"
        string role
        string content
        enum status "enum: enabled/disabled; default: disabled"
        int order "default: 0"
        json settings
        int project_id FK
    }
    prompt_protection_rules {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string name
        string description "default: ''"
        string pattern
        enum status "enum: enabled/disabled/archived; default: disabled"
        json settings
    }
    projects ||--o{ prompts : project_id
```

## Plugins

Plugin installations, schedules and their runs, plugin key-value storage, and table storage.

```mermaid
erDiagram
    plugins {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string plugin_id
        string name "nullable; default: ''"
        string version "nullable; default: ''"
        string description "nullable; default: ''"
        json manifest "nullable"
        bytes package "nullable"
        string package_sha256 "nullable; default: ''"
        bool enabled "default: false"
        int priority "default: 100"
        json granted_permissions "nullable"
        json granted_network "nullable"
        json config "nullable"
    }
    plugin_schedules {
        int id PK
        int installation_id "logical ref"
        string key
        string override_cron "default: ''"
        string override_timezone "default: ''"
        bool paused "default: false"
        int64 last_slot "default: 0"
        int active_run_id "default: 0"
        int64 lease_until "default: 0"
    }
    plugin_schedule_runs {
        int id PK
        int schedule_id "logical ref"
        time scheduled_at "nullable"
        time started_at
        time finished_at "nullable"
        string status
    }
    plugin_storages {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int installation_id "logical ref"
        string namespace "default: default"
        string key
        string value
        time expires_at "nullable"
    }
    plugin_table_storages {
        int id PK
        string plugin_id "logical ref"
        string active_plugin_id UK "nullable"
        json tables
        json names
        int revision "default: 0"
    }
    plugin_table_operations {
        int id PK
        int storage_id "logical ref"
        string kind
        json tables
        json names
        string package_sha "default: ''"
        string last_error "default: ''"
        int64 retry_at "default: 0"
    }
    plugins ||..o{ plugin_schedules : installation_id
    plugins ||..o{ plugin_storages : installation_id
    plugin_schedules ||..o{ plugin_schedule_runs : schedule_id
    plugin_table_storages ||..o{ plugin_table_operations : storage_id
    plugins ||..o{ plugin_table_storages : plugin_id
```

## System settings

Global key-value system settings.

```mermaid
erDiagram
    systems {
        int id PK
        time created_at "default: CURRENT_TIMESTAMP"
        time updated_at "default: CURRENT_TIMESTAMP"
        int deleted_at "default: 0"
        string key UK
        string value
    }
```

## Logical references (no foreign key)

These columns point to other tables, but the database has no foreign key. Business code keeps them consistent on write and delete.

| Table.column | References | Nullable |
|---|---|---|
| `requests.user_id` | `users.id` | Yes |
| `usage_logs.user_id` | `users.id` | Yes |
| `usage_logs.api_key_id` | `api_keys.id` | Yes |
| `request_executions.project_id` | `projects.id` | No |
| `request_body_chunks.project_id` | `projects.id` | No |
| `invitations.role_id` | `roles.id` | Yes |
| `channel_model_price_versions.channel_id` | `channels.id` | No |
| `plugin_schedules.installation_id` | `plugins.id` | No |
| `plugin_storages.installation_id` | `plugins.id` | No |
| `plugin_schedule_runs.schedule_id` | `plugin_schedules.id` | No |
| `plugin_table_operations.storage_id` | `plugin_table_storages.id` | No |
| `plugin_table_storages.plugin_id` | `plugins.plugin_id` | No |

## Table list

| Table | Domain | Columns | References |
|---|---|---|---|
| `users` | Identity and access | 13 | - |
| `projects` | Identity and access | 8 | - |
| `user_projects` | Identity and access | 7 | `projects`, `users` |
| `roles` | Identity and access | 8 | `projects` |
| `user_roles` | Identity and access | 5 | `roles`, `users` |
| `api_keys` | Identity and access | 13 | `projects`, `users` |
| `api_key_profile_templates` | Identity and access | 9 | `projects` |
| `oidc_identities` | Identity and access | 10 | `users` |
| `invitations` | Identity and access | 10 | `projects`, `roles` |
| `channels` | Channels and models | 24 | - |
| `models` | Channels and models | 14 | - |
| `channel_model_prices` | Channels and models | 8 | `channels` |
| `channel_model_price_versions` | Channels and models | 11 | `channel_model_prices`, `channels` |
| `channel_override_templates` | Channels and models | 11 | `users` |
| `channel_probes` | Channels and models | 7 | `channels` |
| `provider_quota_status` | Channels and models | 12 | `channels` |
| `requests` | Requests and usage | 31 | `api_keys`, `channels`, `data_storages`, `projects`, `traces`, `users` |
| `request_executions` | Requests and usage | 29 | `channels`, `data_storages`, `projects`, `requests` |
| `request_body_chunks` | Requests and usage | 5 | `projects` |
| `request_body_chunk_refs` | Requests and usage | 5 | `request_body_chunks`, `request_executions`, `requests` |
| `usage_logs` | Requests and usage | 26 | `api_keys`, `channels`, `projects`, `requests`, `users` |
| `traces` | Requests and usage | 8 | `projects`, `threads` |
| `threads` | Requests and usage | 6 | `projects` |
| `data_storages` | Requests and usage | 10 | - |
| `prompts` | Prompts | 12 | `projects` |
| `prompt_protection_rules` | Prompts | 9 | - |
| `plugins` | Plugins | 16 | - |
| `plugin_schedules` | Plugins | 9 | `plugins` |
| `plugin_schedule_runs` | Plugins | 6 | `plugin_schedules` |
| `plugin_storages` | Plugins | 8 | `plugins` |
| `plugin_table_storages` | Plugins | 6 | `plugins` |
| `plugin_table_operations` | Plugins | 8 | `plugin_table_storages` |
| `systems` | System settings | 6 | - |
