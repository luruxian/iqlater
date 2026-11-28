# iqlater（axonhub 版）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以 fork 的 axonhub 作为唯一统一入口、magpie 作为其上游渠道，搭好本地开发环境（axonhub 不用 Docker），并用 Docker Compose（Caddy + axonhub + magpie + PostgreSQL 18）部署到单台云服务器。

**Architecture:** 本期不改 axonhub 代码。新增 `dev/`（本地 magpie 容器与 axonhub 配置模板）、`deploy/iqlater/`（部署编排、Caddyfile、备份脚本）和 `.github/workflows/iqlater-image.yml`（构建推送 `ghcr.io/luruxian/iqlater`）。magpie 在 axonhub 后台配置为三个渠道（`anthropic`、`openai_responses`、`openai`），Base URL 为 magpie 网关。

**Tech Stack:** axonhub（Go 1.27.4、Gin、Ent、gqlgen；React 19 + pnpm）、magpie `ghcr.io/yetone/magpie:stable`、PostgreSQL 18、Docker Compose、Caddy 2、GitHub Actions + GHCR。

**Spec:** `docs/superpowers/specs/2026-11-27-iqlater-axonhub-design.md`

## Global Constraints

- 仓库：`github.com/luruxian/iqlater`（fork 自 `looplj/axonhub`）；本地路径 `/Users/a123/projects/iqlater`；`upstream` 为 `https://github.com/looplj/axonhub.git`。
- 工作分支 `main`，基于上游发布标签 `v1.0.0-rc.5`，设为 GitHub 默认分支；不在 `unstable` 上提交。
- 本期不修改 axonhub 的 Go / TS 源码与数据模型；保留 `LICENSE`、`NOTICE`、上游根目录 `docker-compose.yml` 与 `deploy/` 下的上游文件不动。
- 不在 `docs/` 下新增除 `docs/superpowers/` 以外的文件（上游规则要求 `docs/` 中英文同步）。
- Go 构建需 Go 1.27.4；本机 1.26.10 依赖 `GOTOOLCHAIN=auto` 自动下载（已验证可用）。后端构建：`go build -tags=nomsgpack -o axonhub ./cmd/axonhub`。
- 本机 `docker` 不在默认 PATH：命令前执行 `export PATH="$PATH:/Applications/Docker.app/Contents/Resources/bin"`。
- magpie 镜像 `ghcr.io/yetone/magpie:stable`；PostgreSQL 部署镜像 `postgres:18`，数据卷挂到 `/var/lib/postgresql`。
- 本地开发 magpie：网络子网 `172.29.0.0/24`，`MAGPIE_ALLOW_IPS=172.29.0.1`，端口 `127.0.0.1:3425`（网关）、`127.0.0.1:3430`（Web UI）。
- 部署：项目名 `iqlater`；`edge` 网络子网 `172.28.0.0/24`；axonhub `AXONHUB_SERVER_TRUSTED_PROXIES='["172.28.0.0/24"]'`；magpie `MAGPIE_ALLOW_IPS=172.28.0.0/24`。
- 镜像：`ghcr.io/luruxian/iqlater:<tag>`，tag 形如 `iqlater-v0.1.0`。
- axonhub 渠道 Base URL 不带 `/v1`（axonhub 自动补）：开发 `http://127.0.0.1:3425`，部署 `http://magpie:3425`。
- 不提交任何密钥：`config.yml`、`.env` 已被仓库 `.gitignore` 忽略；模板分别命名为 `config.example.yml`、`.env.example`。

## 前置条件（已确认）

- Docker Desktop 29.9.3（Compose v5.7.1）运行中。
- 宿主机 PostgreSQL 18.7（Homebrew，`brew services` 自启），`localhost:5432`，当前用户 `a123` 免密。
- pnpm 与 Node 26 已安装；`gh` 已登录 `luruxian`（`repo`、`workflow`、`delete_repo` 权限）。

## 文件结构

| 文件 | 职责 |
| :-- | :-- |
| `dev/docker-compose.yml` | 本地开发用的 magpie 容器 |
| `dev/.env.example` | 本地 magpie 的 `MAGPIE_WEB_KEY` 模板 |
| `dev/config.example.yml` | 本机运行 axonhub 的配置模板（复制为根目录 `config.yml`） |
| `deploy/iqlater/docker-compose.yml` | 部署编排：caddy、axonhub、magpie、postgres |
| `deploy/iqlater/docker-compose.local.yml` | 本机用本地构建镜像验证部署编排（不启动 Caddy） |
| `deploy/iqlater/Caddyfile` | HTTPS 反向代理 |
| `deploy/iqlater/.env.example` | 部署变量模板 |
| `deploy/iqlater/scripts/backup.sh` | 加密备份 PG 与 magpie 配置卷 |
| `.github/workflows/iqlater-image.yml` | 测试并构建推送 GHCR 镜像 |

---

### Task 1: 建立 `main` 分支并整理 fork

**Files:**
- 已存在、需提交：`docs/superpowers/specs/2026-11-27-iqlater-axonhub-design.md`、`docs/superpowers/plans/2026-11-27-iqlater-axonhub.md`

**Interfaces:**
- Consumes: 无
- Produces: 远程默认分支 `main`（基于 `v1.0.0-rc.5`）；上游发布 / 定时类 workflow 已停用

- [ ] **Step 1: 从发布标签创建 `main` 并推送**

工作区中未跟踪的 `docs/superpowers/` 会随分支切换保留。

```bash
cd /Users/a123/projects/iqlater
git fetch upstream --tags
git switch -c main v1.0.0-rc.5
git push -u origin main
gh repo edit luruxian/iqlater --default-branch main
gh repo view luruxian/iqlater --json defaultBranchRef --jq .defaultBranchRef.name
```

Expected: 最后一行输出 `main`

- [ ] **Step 2: 停用上游发布与定时类 workflow**

`test.yml`、`lint.yml` 只在 `unstable` / `release/**` 上触发，保留；`changes.yml` 只被调用，保留。

```bash
for wf in build.yml community-plugins.yml desktop-release.yml docker-build.yml docker-pr.yml \
          docker-publish.yml docker-unstable.yml helm-chart.yml release.yml stale.yml \
          sync-model-developers.yml vulncheck.yml; do
  gh workflow disable "$wf" -R luruxian/iqlater
done
gh workflow list -R luruxian/iqlater --all
```

Expected: 上述 12 个为 `disabled_manually`；`Test`、`Lint`（及 `changes`）仍为 `active`

- [ ] **Step 3: 提交设计文档与计划**

```bash
git add docs/superpowers
git commit -m "docs: add iqlater design spec and implementation plan"
git push
```

---

### Task 2: 本地开发环境（axonhub 本机运行 + magpie 容器 + 宿主机 PG）

**Files:**
- Create: `dev/docker-compose.yml`
- Create: `dev/.env.example`
- Create: `dev/config.example.yml`

**Interfaces:**
- Consumes: Task 1 的 `main` 分支
- Produces: 本地 magpie 网关 `http://127.0.0.1:3425`、Web UI `http://127.0.0.1:3430`；axonhub 后端 `http://localhost:8090`、前端 `http://localhost:5173`；PG 库 `iqlater`、用户 `iqlater`

- [ ] **Step 1: 写 `dev/docker-compose.yml`**

```yaml
# 本地开发：只有 magpie 跑在 Docker 里，axonhub 与 PostgreSQL 在宿主机上运行。
# 启动：docker compose -f dev/docker-compose.yml up -d
# Web UI 地址（含 key）：docker compose -f dev/docker-compose.yml logs magpie | grep -o 'http://127.0.0.1:3430/[^ ]*'
name: iqlater-dev

services:
  magpie:
    image: ghcr.io/yetone/magpie:stable
    restart: unless-stopped
    command: [web, --addr, "0.0.0.0:3430", --no-open]
    ports:
      - "127.0.0.1:3425:3425"
      - "127.0.0.1:3430:3430"
    environment:
      MAGPIE_WEB_KEY: ${MAGPIE_WEB_KEY:?set MAGPIE_WEB_KEY in dev/.env}
      MAGPIE_NAME: iqlater-dev
      # Docker Desktop 把经端口映射进来的请求的来源地址改写为本网络的网关
      MAGPIE_ALLOW_IPS: 172.29.0.1
      TZ: Asia/Shanghai
    volumes:
      - magpie_config:/config
    healthcheck:
      test: ["CMD", "/magpie", "healthcheck"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 30s
    networks: [magpie]

networks:
  magpie:
    ipam:
      config:
        - subnet: 172.29.0.0/24
          gateway: 172.29.0.1

volumes:
  magpie_config:
```

- [ ] **Step 2: 写 `dev/.env.example`**

```bash
# 复制为 dev/.env（已被 .gitignore 忽略）。生成：openssl rand -hex 24
MAGPIE_WEB_KEY=
```

- [ ] **Step 3: 写 `dev/config.example.yml`**

```yaml
# 本机运行 axonhub 的配置。复制为仓库根目录的 config.yml（已被 .gitignore 忽略），
# 把 CHANGE_ME 换成 iqlater 数据库用户的密码，然后在仓库根目录运行 ./axonhub。
server:
  port: 8090
  name: "iqlater"

db:
  dialect: "postgres"
  dsn: "postgres://iqlater:CHANGE_ME@localhost:5432/iqlater?sslmode=disable"

log:
  level: "info"
```

- [ ] **Step 4: 校验 compose 文件**

```bash
export PATH="$PATH:/Applications/Docker.app/Contents/Resources/bin"
MAGPIE_WEB_KEY=x docker compose -f dev/docker-compose.yml config --quiet && echo dev-compose-ok
```

Expected: `dev-compose-ok`

- [ ] **Step 5: 提交**

```bash
git add dev/
git commit -m "chore(dev): add local magpie container and axonhub config template"
```

- [ ] **Step 6: 在宿主机 PG 中建库与用户**

```bash
PGPW=$(openssl rand -hex 16)
psql -d postgres -v ON_ERROR_STOP=1 \
  -c "CREATE ROLE iqlater LOGIN PASSWORD '$PGPW'" \
  -c "CREATE DATABASE iqlater OWNER iqlater"
sed "s/CHANGE_ME/$PGPW/" dev/config.example.yml > config.yml
psql "postgres://iqlater:$PGPW@localhost:5432/iqlater" -Atc "select current_user, current_database()"
```

Expected: `iqlater|iqlater`

- [ ] **Step 7: 启动本地 magpie 并验证访问策略**

```bash
printf 'MAGPIE_WEB_KEY=%s\n' "$(openssl rand -hex 24)" > dev/.env
docker compose -f dev/docker-compose.yml up -d
until [ "$(docker inspect -f '{{.State.Health.Status}}' iqlater-dev-magpie-1)" = healthy ]; do sleep 3; done
curl -s http://127.0.0.1:3425/v1/models
```

Expected: 返回 401，消息含 `no API key was sent`（来源地址已被放行，只缺 gateway key；若为 403 且消息里的地址不是 `172.29.0.1`，把该地址写入 `MAGPIE_ALLOW_IPS` 后重来）

- [ ] **Step 8: 构建并启动 axonhub 后端**

```bash
go build -tags=nomsgpack -o axonhub ./cmd/axonhub
./axonhub > /tmp/axonhub-dev.log 2>&1 &
until curl -sf http://localhost:8090/health >/dev/null; do sleep 2; done
curl -s http://localhost:8090/admin/system/status
```

Expected: `{"isInitialized":false}`（PG 中已由自动迁移建表：`psql "postgres://iqlater:$PGPW@localhost:5432/iqlater" -Atc "select count(*) from information_schema.tables where table_schema='public'"` 大于 0）

- [ ] **Step 9: 启动前端并初始化系统**

```bash
(cd frontend && pnpm install --frozen-lockfile && pnpm dev > /tmp/axonhub-frontend.log 2>&1 &)
```

浏览器打开 `http://localhost:5173`，按初始化页面填写：品牌名 `iqlater`、管理员邮箱与密码（由用户自定）。完成后：

```bash
curl -s http://localhost:8090/admin/system/status
```

Expected: `{"isInitialized":true}`；能用管理员账号登录后台

---

### Task 3: 本地把 magpie 接为 axonhub 渠道

本任务需要用户在 magpie Web UI 中登录订阅账号，由控制方与用户一起完成，不产生提交。

**Interfaces:**
- Consumes: Task 2 的本地 magpie、axonhub
- Produces: axonhub 中三个指向 magpie 的渠道；一个 axonhub API key；经 axonhub 的三种协议调用均成功

- [ ] **Step 1: 在 magpie 中登录至少一个上游**

```bash
docker compose -f dev/docker-compose.yml logs magpie | grep -o 'http://127.0.0.1:3430/[^ ]*' | tail -1
```

浏览器打开该地址：
- Claude 订阅：先执行 `docker compose -f dev/docker-compose.yml exec magpie magpie claude-code install`，再在 Web UI 中 OAuth 登录（粘贴授权码）。
- ChatGPT / Codex 订阅：Web UI 中 OAuth 登录，把浏览器跳转失败的 `http://localhost:1455/...` 地址粘回。
- 按 key 的套餐（如 DeepSeek、GLM）：Web UI 中添加 provider 并粘贴 key。

- [ ] **Step 2: 生成 magpie gateway key**

```bash
docker compose -f dev/docker-compose.yml exec magpie magpie gateway-key add iqlater
```

Expected: 输出形如 `sk-magpie-key-<48 位十六进制>`。验证：

```bash
MAGPIE_KEY=<上面的 key>
curl -s http://127.0.0.1:3425/v1/models -H "Authorization: Bearer $MAGPIE_KEY" | head -c 400
```

Expected: `{"object":"list","data":[...]}`，含已登录上游的模型

- [ ] **Step 3: 在 axonhub 后台建三个渠道**

后台 → 渠道 → 新建，每个渠道：Base URL `http://127.0.0.1:3425`，API Key 为 `MAGPIE_KEY`，用渠道表单的"获取模型"从 magpie 拉取并勾选模型，开启"自动同步支持的模型"。

| 名称 | 类型 | 勾选的模型 |
| :-- | :-- | :-- |
| `magpie-anthropic` | `anthropic` | Claude 系列 |
| `magpie-responses` | `openai_responses` | GPT / Codex 系列 |
| `magpie-openai` | `openai` | 其余按 key 套餐的模型 |

逐个点"测试"，Expected: 测试通过

- [ ] **Step 4: 建 axonhub API key 并经 axonhub 调用**

后台 → API Keys → 新建（名称 `dev`），记下 key 为 `AH_KEY`。按已登录的上游选择对应调用（`<model>` 用渠道中勾选的模型名）：

```bash
# Anthropic Messages
curl -s http://localhost:8090/v1/messages -H "x-api-key: $AH_KEY" -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  -d '{"model":"<claude 模型>","max_tokens":32,"messages":[{"role":"user","content":"Say hi"}]}'
# OpenAI Responses
curl -s http://localhost:8090/v1/responses -H "Authorization: Bearer $AH_KEY" -H 'content-type: application/json' \
  -d '{"model":"<gpt 模型>","input":"Say hi"}'
# OpenAI Chat Completions
curl -s http://localhost:8090/v1/chat/completions -H "Authorization: Bearer $AH_KEY" -H 'content-type: application/json' \
  -d '{"model":"<其他模型>","messages":[{"role":"user","content":"Say hi"}]}'
```

Expected: 已登录上游对应的调用返回正常回复；axonhub 后台"请求"页能看到这些请求与用量；magpie Web UI 中对应账号的用量增加

---

### Task 4: 部署编排与本机验证

**Files:**
- Create: `deploy/iqlater/docker-compose.yml`
- Create: `deploy/iqlater/docker-compose.local.yml`
- Create: `deploy/iqlater/Caddyfile`
- Create: `deploy/iqlater/.env.example`
- Create: `deploy/iqlater/scripts/backup.sh`

**Interfaces:**
- Consumes: 仓库根目录 `Dockerfile`（构建 axonhub 镜像，入口 `/app/axonhub`，健康检查 `GET /health`）
- Produces: compose 服务名 `caddy`、`axonhub`、`magpie`、`postgres`；卷 `iqlater_postgres_data`、`iqlater_magpie_config`、`iqlater_caddy_data`、`iqlater_caddy_config`；Task 6 在服务器上使用这些文件

- [ ] **Step 1: 写 `deploy/iqlater/docker-compose.yml`**

```yaml
# iqlater 部署：Caddy（HTTPS）→ axonhub（统一入口）→ magpie（上游渠道）；PostgreSQL 仅 axonhub 可达。
# 用法（在本目录）：cp .env.example .env 并填写，然后 docker compose up -d
name: iqlater

services:
  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    environment:
      DOMAIN: ${DOMAIN:?set DOMAIN in .env}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      axonhub:
        condition: service_healthy
    stop_grace_period: 35s
    networks: [edge]

  axonhub:
    image: ${IQLATER_IMAGE:-ghcr.io/luruxian/iqlater}:${IQLATER_VERSION:?set IQLATER_VERSION in .env}
    restart: unless-stopped
    environment:
      AXONHUB_SERVER_NAME: iqlater
      AXONHUB_SERVER_TRUSTED_PROXIES: '["172.28.0.0/24"]'
      AXONHUB_DB_DIALECT: postgres
      AXONHUB_DB_DSN: postgres://iqlater:${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}@postgres:5432/iqlater?sslmode=disable
      TZ: Asia/Shanghai
    user: "65532:65532"
    init: true
    read_only: true
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev,size=64m
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    depends_on:
      postgres:
        condition: service_healthy
      magpie:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8090/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    # axonhub 的进程停止超时为 30 秒
    stop_grace_period: 35s
    logging:
      driver: json-file
      options: {max-size: 10m, max-file: "3"}
    networks: [edge, backend]

  magpie:
    image: ghcr.io/yetone/magpie:stable
    restart: unless-stopped
    command: [web, --addr, "0.0.0.0:3430", --no-open]
    ports:
      # Web UI 只给 SSH 隧道用：ssh -L 3430:127.0.0.1:3430 <服务器>
      - "127.0.0.1:3430:3430"
    environment:
      MAGPIE_WEB_KEY: ${MAGPIE_WEB_KEY:?set MAGPIE_WEB_KEY in .env}
      MAGPIE_NAME: iqlater
      # axonhub 与 SSH 隧道（经网关 172.28.0.1 进入）都在 edge 子网内
      MAGPIE_ALLOW_IPS: 172.28.0.0/24
      MAGPIE_DRAIN: "300"
      TZ: Asia/Shanghai
    volumes:
      - magpie_config:/config
    healthcheck:
      test: ["CMD", "/magpie", "healthcheck"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 30s
    stop_grace_period: 310s
    networks: [edge]

  postgres:
    image: postgres:18
    restart: unless-stopped
    environment:
      POSTGRES_DB: iqlater
      POSTGRES_USER: iqlater
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}
    volumes:
      - postgres_data:/var/lib/postgresql
    shm_size: 256m
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U iqlater -d iqlater"]
      interval: 10s
      timeout: 5s
      retries: 5
    stop_grace_period: 60s
    networks: [backend]

networks:
  edge:
    ipam:
      config:
        - subnet: 172.28.0.0/24
  backend:
    internal: true

volumes:
  postgres_data:
  magpie_config:
  caddy_data:
  caddy_config:
```

- [ ] **Step 2: 写 `deploy/iqlater/docker-compose.local.yml`**

```yaml
# 在本机用本地构建的镜像验证部署编排，不启动 Caddy。
#   docker build -t iqlater:local ../..
#   docker compose -f docker-compose.yml -f docker-compose.local.yml up -d axonhub
# 先停掉 dev/ 的 magpie 和本机 axonhub，避免 3430、8090 端口冲突。
services:
  axonhub:
    image: iqlater:local
    ports:
      - "127.0.0.1:8090:8090"
```

- [ ] **Step 3: 写 `deploy/iqlater/Caddyfile`**

```caddyfile
{$DOMAIN} {
	reverse_proxy axonhub:8090 {
		# 流式响应（SSE）逐块转发
		flush_interval -1
	}
}
```

- [ ] **Step 4: 写 `deploy/iqlater/.env.example`**

```bash
# 复制为 .env（已被 .gitignore 忽略）后填写。随机值用：openssl rand -hex 24
DOMAIN=iqlater.example.com
IQLATER_VERSION=iqlater-v0.1.0
POSTGRES_PASSWORD=
MAGPIE_WEB_KEY=
# 备份加密口令（务必另行保存，丢失则备份无法解密）
BACKUP_PASSPHRASE=
# 可选：rclone 远程，如 r2:iqlater-backups；留空则只保留本地备份
BACKUP_RCLONE_REMOTE=
```

- [ ] **Step 5: 写 `deploy/iqlater/scripts/backup.sh` 并设为可执行**

```bash
#!/usr/bin/env bash
# Daily backup of the iqlater database and the magpie config volume.
# cron: 30 4 * * * /opt/iqlater/deploy/iqlater/scripts/backup.sh >> /var/log/iqlater-backup.log 2>&1
#
# Restore (stop magpie and axonhub first; never run a restored magpie volume
# beside the original, both would refresh the same sign-ins):
#   openssl enc -d -aes-256-cbc -pbkdf2 -pass env:BACKUP_PASSPHRASE -in backups/pg-<stamp>.dump.enc \
#     | docker compose exec -T postgres pg_restore -U iqlater -d iqlater --clean --if-exists
#   openssl enc -d -aes-256-cbc -pbkdf2 -pass env:BACKUP_PASSPHRASE -in backups/magpie-<stamp>.tar.gz.enc \
#     | docker run --rm -i -v iqlater_magpie_config:/config alpine:3 tar -C /config -xzf -
#   The magpie archive excludes /config/cache, so after restoring run:
#     docker compose exec magpie magpie claude-code install
set -euo pipefail

cd "$(dirname "$0")/.."
set -a
# shellcheck disable=SC1091
source .env
set +a
: "${BACKUP_PASSPHRASE:?BACKUP_PASSPHRASE is not set in deploy/iqlater/.env}"

stamp=$(date +%Y%m%d-%H%M%S)
dir=backups
keep=7
umask 077
mkdir -p "$dir"

pg_out="$dir/pg-$stamp.dump.enc"
pg_tmp="${pg_out}.tmp"
magpie_out="$dir/magpie-$stamp.tar.gz.enc"
magpie_tmp="${magpie_out}.tmp"
# a failed run must not leave half-written encrypted files behind
trap 'rm -f -- "$pg_tmp" "$magpie_tmp"' EXIT
docker compose exec -T postgres pg_dump -U iqlater -d iqlater --format=custom \
  | openssl enc -aes-256-cbc -pbkdf2 -salt -pass env:BACKUP_PASSPHRASE -out "$pg_tmp"
mv "$pg_tmp" "$pg_out"

# ./cache is Bun, the model catalog and other re-downloadable data
docker run --rm -v iqlater_magpie_config:/config:ro alpine:3 tar -C /config --exclude=./cache -czf - . \
  | openssl enc -aes-256-cbc -pbkdf2 -salt -pass env:BACKUP_PASSPHRASE -out "$magpie_tmp"
mv "$magpie_tmp" "$magpie_out"

for prefix in pg magpie; do
  ls -1t "$dir"/"$prefix"-*.enc 2>/dev/null | tail -n +$((keep + 1)) | xargs -r rm --
done

if [ -n "${BACKUP_RCLONE_REMOTE:-}" ]; then
  # Remote copies are not pruned by this script; manage retention on the rclone destination separately.
  rclone copy "$dir" "$BACKUP_RCLONE_REMOTE" --include "*-$stamp.*"
fi

echo "backup $stamp done"
```

```bash
chmod +x deploy/iqlater/scripts/backup.sh
printf 'backups/\n' > deploy/iqlater/.gitignore
```

- [ ] **Step 6: 静态校验**

```bash
cd deploy/iqlater
cp .env.example .env.check
for k in POSTGRES_PASSWORD MAGPIE_WEB_KEY; do sed -i '' "s/^$k=$/$k=x/" .env.check; done
docker compose --env-file .env.check config --quiet && echo compose-ok
docker compose --env-file .env.check -f docker-compose.yml -f docker-compose.local.yml config --quiet && echo local-ok
DOMAIN=example.com caddy validate --config Caddyfile --adapter caddyfile 2>&1 | tail -1
bash -n scripts/backup.sh && echo backup-syntax-ok
rm .env.check
```

Expected: `compose-ok`、`local-ok`、`Valid configuration`、`backup-syntax-ok`

- [ ] **Step 7: 提交**

```bash
cd /Users/a123/projects/iqlater
git add deploy/iqlater
git commit -m "feat(deploy): add docker compose stack for axonhub, magpie and postgres"
```

- [ ] **Step 8: 本机用本地镜像起部署编排**

先停掉 Task 2 的本地进程与容器（`pkill -f '^./axonhub'`、`docker compose -f dev/docker-compose.yml down`，保留卷）。

```bash
cd /Users/a123/projects/iqlater
docker build -t iqlater:local .
cd deploy/iqlater
{ echo "DOMAIN=localhost"; echo "IQLATER_VERSION=local"; \
  echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"; echo "MAGPIE_WEB_KEY=$(openssl rand -hex 24)"; \
  echo "BACKUP_PASSPHRASE=$(openssl rand -hex 24)"; echo "BACKUP_RCLONE_REMOTE="; } > .env
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d axonhub
until [ "$(docker inspect -f '{{.State.Health.Status}}' iqlater-axonhub-1)" = healthy ]; do sleep 3; done
docker compose ps --format '{{.Service}} {{.Status}}'
curl -s http://127.0.0.1:8090/admin/system/status
```

Expected: `axonhub`、`magpie`、`postgres` 均为 `Up ... (healthy)`，无 `caddy`；状态 `{"isInitialized":false}`

- [ ] **Step 9: 在容器编排里重复渠道验证**

1. `curl -s -X POST http://127.0.0.1:8090/admin/system/initialize -H 'content-type: application/json' -d '{"ownerEmail":"admin@iqlater.local","ownerPassword":"<自定>","ownerFirstName":"Admin","ownerLastName":"iqlater","brandName":"iqlater","preferLanguage":"zh"}'`，Expected: `"success":true`。
2. 在部署编排的 magpie 中添加一个按 key 的上游（经 `http://127.0.0.1:3430` Web UI），`docker compose exec magpie magpie gateway-key add iqlater` 生成 key。
3. 后台 `http://127.0.0.1:8090` 登录，按 Task 3 Step 3 建 `magpie-openai` 渠道，Base URL 改为 `http://magpie:3425`；建 API key 后用 Task 3 Step 4 的 Chat Completions 调用。

Expected: 调用成功；`docker compose exec postgres psql -U iqlater -d iqlater -Atc "select count(*) from requests"` 大于 0

- [ ] **Step 10: 备份与恢复演练**

```bash
./scripts/backup.sh
ls backups/
openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:$(grep ^BACKUP_PASSPHRASE= .env | cut -d= -f2)" \
  -in "$(ls -1t backups/magpie-*.enc | head -1)" | tar -tzf - | grep -c '^./cache/' || true
docker compose exec magpie ls /config/magpie/artifacts 2>/dev/null | wc -l
```

Expected: `backup <stamp> done`；`backups/` 下各一个 `pg-*.dump.enc`、`magpie-*.tar.gz.enc`；cache 计数为 `0`；artifacts 为空（`0`）。
然后执行脚本头部的 PG 恢复命令（先 `docker compose stop axonhub`），再 `docker compose -f docker-compose.yml -f docker-compose.local.yml up -d axonhub`，后台渠道与 API key 仍在、调用仍成功。

- [ ] **Step 11: 清理本机部署编排**

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml down -v
rm -rf .env backups
```

Expected: `docker volume ls | grep iqlater_` 无输出；`git status --short` 干净

---

### Task 5: GHCR 镜像 workflow

**Files:**
- Create: `.github/workflows/iqlater-image.yml`

**Interfaces:**
- Consumes: 仓库根目录 `Dockerfile`；`deploy/iqlater/migrate/Dockerfile`；`.github/workflows/iqlater-migrations.yml`；`make generate`、`make test-backend-all`
- Produces: 推送 `iqlater-v*` 标签后得到 `ghcr.io/luruxian/iqlater:<标签>` 与 `:latest`，以及 `ghcr.io/luruxian/iqlater-migrate:<标签>` 与 `:latest`（迁移检查由 `iqlater-migrations.yml` 提供）

- [ ] **Step 1: 写 workflow**

action 的 commit SHA 取自上游 `.github/workflows/docker-build.yml`；`actions/setup-go` 与上游 `test.yml` 一致使用 `v7.1.0` 标签。

```yaml
name: iqlater image

on:
  push:
    tags:
      - 'iqlater-v*'
  # 手动触发只跑 test，不推镜像
  workflow_dispatch:

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@v7.1.0
        with:
          go-version: '1.27.4'
      - name: Generate GraphQL and Ent code
        run: make generate
      - name: Test
        run: make test-backend-all

  migrations:
    uses: ./.github/workflows/iqlater-migrations.yml

  image:
    needs: [test, migrations]
    if: startsWith(github.ref, 'refs/tags/iqlater-v')
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - name: Write version
        run: printf '%s' "${GITHUB_REF_NAME}" > internal/build/VERSION
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          platforms: linux/amd64
          push: true
          tags: |
            ghcr.io/luruxian/iqlater:${{ github.ref_name }}
            ghcr.io/luruxian/iqlater:latest
          labels: org.opencontainers.image.source=https://github.com/luruxian/iqlater
          cache-from: type=gha
          cache-to: type=gha,mode=max
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          file: deploy/iqlater/migrate/Dockerfile
          platforms: linux/amd64
          push: true
          tags: |
            ghcr.io/luruxian/iqlater-migrate:${{ github.ref_name }}
            ghcr.io/luruxian/iqlater-migrate:latest
          labels: org.opencontainers.image.source=https://github.com/luruxian/iqlater
          cache-from: type=gha,scope=migrate
          cache-to: type=gha,scope=migrate,mode=max
```

- [ ] **Step 2: 校验 YAML 并核对 SHA**

```bash
ruby -ryaml -e 'YAML.load_file(ARGV[0]); puts "yaml-ok"' .github/workflows/iqlater-image.yml
for a in actions/checkout docker/setup-buildx-action docker/login-action docker/build-push-action; do
  grep -ho "$a@[0-9a-f]\{40\}" .github/workflows/docker-build.yml | sort -u
done
```

Expected: `yaml-ok`；列出的 4 个 SHA 与新 workflow 中一致

- [ ] **Step 3: 提交、推送并手动触发一次**

```bash
git add .github/workflows/iqlater-image.yml
git commit -m "ci: build and push the iqlater image to GHCR"
git push
gh workflow run iqlater-image.yml -R luruxian/iqlater --ref main
sleep 10
gh run watch "$(gh run list -R luruxian/iqlater --workflow iqlater-image.yml --limit 1 --json databaseId --jq '.[0].databaseId')" \
  -R luruxian/iqlater --exit-status
```

Expected: `test`、`migrations` 成功，`image` 为 skipped

---

### Task 6: 发布镜像并部署到服务器

需要用户提供：服务器 SSH 地址（已装 Docker 与 Compose 插件）、已解析到该服务器的域名。

**Interfaces:**
- Consumes: Task 4 的 `deploy/iqlater/`；Task 5 的 workflow
- Produces: `https://<DOMAIN>` 上运行的 iqlater

- [ ] **Step 1: 打标签发布镜像并公开 GHCR 包**

```bash
cd /Users/a123/projects/iqlater
git tag iqlater-v0.1.0 && git push origin iqlater-v0.1.0
sleep 10
gh run watch "$(gh run list -R luruxian/iqlater --workflow iqlater-image.yml --limit 1 --json databaseId --jq '.[0].databaseId')" \
  -R luruxian/iqlater --exit-status
gh api -X PATCH /user/packages/container/iqlater -f visibility=public
docker manifest inspect ghcr.io/luruxian/iqlater:iqlater-v0.1.0 >/dev/null && echo image-public
```

Expected: `test`、`image` 均成功；`image-public`（若 PATCH 不被接受，在 GitHub 包设置页手动改为 Public）

- [ ] **Step 2: 把部署文件放到服务器并填写 `.env`**

```bash
ssh <服务器> 'sudo mkdir -p /opt/iqlater && sudo chown $USER /opt/iqlater && git clone https://github.com/luruxian/iqlater.git /opt/iqlater/src'
ssh <服务器> 'ln -sfn /opt/iqlater/src/deploy /opt/iqlater/deploy && cd /opt/iqlater/deploy/iqlater && cp .env.example .env'
```

在服务器上编辑 `/opt/iqlater/deploy/iqlater/.env`：`DOMAIN`、`IQLATER_VERSION=iqlater-v0.1.0`，`POSTGRES_PASSWORD`、`MAGPIE_WEB_KEY`、`BACKUP_PASSPHRASE` 用 `openssl rand -hex 24` 生成；`BACKUP_PASSPHRASE` 另行保存。

- [ ] **Step 3: 启动**

```bash
ssh <服务器> 'cd /opt/iqlater/deploy/iqlater && docker compose pull && docker compose up -d && docker compose ps --format "{{.Service}} {{.Status}}"'
curl -sI https://<DOMAIN>/health | head -1
```

Expected: 四个服务均 `Up`（axonhub、magpie、postgres 为 healthy）；`HTTP/2 200`

- [ ] **Step 4: 初始化与接入 magpie**

1. 浏览器打开 `https://<DOMAIN>`，按初始化页面创建管理员（品牌名 `iqlater`）。
2. 本机 `ssh -L 3430:127.0.0.1:3430 <服务器>`，服务器上 `docker compose logs magpie | grep -o 'http://127.0.0.1:3430/[^ ]*' | tail -1` 取地址，在本机浏览器打开，登录各订阅（Claude 先 `docker compose exec magpie magpie claude-code install`）。
3. `docker compose exec magpie magpie gateway-key add iqlater` 生成 key。
4. 后台按 Task 3 Step 3 建三个渠道，Base URL 为 `http://magpie:3425`；建 API key，用 Task 3 Step 4 的命令（把 `http://localhost:8090` 换成 `https://<DOMAIN>`）验证。

Expected: 三种协议中已登录上游对应的调用成功

- [ ] **Step 5: 定时备份**

```bash
ssh <服务器> '/opt/iqlater/deploy/iqlater/scripts/backup.sh && (crontab -l 2>/dev/null; echo "30 4 * * * /opt/iqlater/deploy/iqlater/scripts/backup.sh >> /var/log/iqlater-backup.log 2>&1") | crontab -'
```

Expected: `backup <stamp> done`；`crontab -l` 含该行（`/var/log/iqlater-backup.log` 需对当前用户可写，否则改为 `$HOME/iqlater-backup.log`）

---

## 跟进上游（日后例行操作）

```bash
git fetch upstream --tags
git switch main
git merge <新的上游发布标签，如 v1.0.0>
# 解决冲突（本项目只新增 dev/、deploy/iqlater/、docs/superpowers/、.github/workflows/iqlater-image.yml，通常无冲突）
git push
git tag iqlater-v<版本> && git push origin iqlater-v<版本>
```

上游若新增 workflow，检查其触发条件，必要时 `gh workflow disable`。
