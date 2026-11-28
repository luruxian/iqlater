# 迁移镜像：在 axonhub 启动前对 PostgreSQL 应用 Atlas 版本化迁移。
# 构建上下文为仓库根目录：docker build -f deploy/iqlater/migrate/Dockerfile -t iqlater-migrate:local .
# 使用开源（community）构建；升级版本时同步更新此处的固定标签。
FROM arigaio/atlas:1.4.0-community

COPY internal/ent/migrate/versioned/postgres/ /migrations/
COPY deploy/iqlater/migrate/atlas.hcl /atlas.hcl

WORKDIR /
CMD ["migrate", "apply", "--env", "deploy"]
