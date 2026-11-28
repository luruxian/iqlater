# iqlater Database Migration Guide

The iqlater production environment (PostgreSQL 18, Docker Compose, `deploy/iqlater/`) manages the schema with Atlas versioned migrations. For the design background, see `docs/superpowers/specs/2026-11-28-iqlater-atlas-migrations-design.md`.

---

## 1. The two migration modes

Selected by the environment variable `AXONHUB_DB_MIGRATION_MODE` (config key `db.migration_mode`):

| Mode | Behavior at startup | Used for |
|------|---------------------|----------|
| `auto` (default) | cleanup → `Schema.Create` → `datamigrate`, same as upstream | Local dev databases, SQLite, tests |
| `versioned` | Skips cleanup and `Schema.Create`; runs only `datamigrate`. The schema is owned by the `migrate` service (Atlas) | iqlater production (set in compose) |

- `disable_auto_migration: true` always wins: everything is skipped, including `datamigrate`.
- Any `migration_mode` other than `auto` or `versioned` fails startup.
- Migration files live in `internal/ent/migrate/versioned/postgres/` (`<timestamp>_<name>.sql` + `atlas.sum`). `internal/ent/migrate/migrations/` is an upstream legacy directory and is unrelated.
- On a fresh database axonhub logs `system not initialized, skipping migration`. This is expected: `datamigrate` runs on later upgrades, once the system has been initialized.

## 2. Day-to-day schema changes

1. Edit `internal/ent/schema/` and run `make generate`.
2. Run `make migrate-diff name=<name>`. It writes a new SQL file into the migration directory and updates `atlas.sum`.
   - By default it starts a throwaway `postgres:18` container and removes it afterwards, so Docker is required.
   - Alternatively set `MIGRATE_DEV_URL` to an empty PostgreSQL database. The DSN must include `search_path=public`, for example `postgres://postgres:pass@127.0.0.1:5432/dev?sslmode=disable&search_path=public`.
3. Review the generated SQL by hand. Look closely at `DROP`, type changes and new NOT NULL columns.
4. Run `make migrate-check`: it confirms the schema matches the migration directory (regenerating produces no new file) and validates `atlas.sum`.
5. Commit the migration files in the same commit as the schema change.

Rules:

- Committed migration files are never edited. Fix mistakes with a new migration file.
- Drop or rename columns in two releases (expand-contract): the first release adds the new column and backfills it; the second drops the old column once no code uses it.
- Migrations only go forward (no down). Rollback relies on the backup taken before the upgrade.

## 3. Syncing with upstream

1. Merge upstream.
2. If upstream changed `internal/ent/schema/` or `schemahook`: run `make generate`, then `make migrate-diff name=upstream_<version>`.
3. Check upstream changes to the pre-DDL cleanup logic in `internal/server/db/ent.go` (such as `cleanupRemovedProviderQuotaTypes`). `versioned` mode does not run these cleanups, so rewrite them as SQL and put it in the new migration file before the DDL.
4. Review the generated SQL, focusing on `DROP`, type changes and new NOT NULL columns.
5. Commit, and release only after CI passes.

Note: Ent's diff does not run Creator hooks registered through `schema.WithHooks` (for example `schemahook.V0_3_0`). This is harmless today, and the CI equivalence test guards it. If upstream adds a new `schemahook`, check the result of that test.

## 4. Release and upgrade

Run all commands from the server's `deploy/iqlater` directory (where the compose project lives).

1. Take a backup first (see `deploy/iqlater/scripts/backup.sh`). Atlas's revision table `atlas_schema_revisions` lives in the `public` schema and is included in `pg_dump` backups. Do not drop it.
2. Change `IQLATER_VERSION` in `.env`. The axonhub and `migrate` images share the same tag, so migrations always match the code version.
3. Pull the images:

   ```bash
   docker compose pull
   ```

4. Preview the migration:

   ```bash
   docker compose run --rm migrate migrate status --env deploy
   docker compose run --rm migrate migrate apply --env deploy --dry-run
   ```

5. Start:

   ```bash
   docker compose up -d
   ```

   The `migrate` service runs `migrate apply --env deploy` first. axonhub starts only if it succeeds. A failed migration, including an `atlas.sum` checksum failure, keeps axonhub from starting.

Notes:

- The `migrate` service runs as user `65532:65532`. The `atlas` binary has mode `0111`, so root cannot execute it once `cap_drop: ALL` is applied.
- The connection string is passed through the `DATABASE_URL` environment variable. The `deploy` environment in `atlas.hcl` reads it, and the migration directory is `/migrations` inside the image.

## 5. Handling failures

- **A migration fails, or the system misbehaves after an upgrade**: restore the database from the pre-upgrade backup and revert `IQLATER_VERSION` in `.env`. There is no automatic rollback.
- **`connected database is not clean`**: Atlas refuses to migrate a non-empty database that has no revision history. If the deploy database was ever created in `auto` mode (for example a local compose rehearsal or a trial start on the server), delete the `postgres_data` volume before the first `versioned` deploy so that it starts from an empty database:

  ```bash
  docker compose down
  docker volume rm iqlater_postgres_data
  ```

  Only do this when there is no data to keep. The production database holds no live data, which is why no baseline-marking procedure was designed. Use the volume name shown by `docker volume ls`.

## 6. Use hex characters for POSTGRES_PASSWORD

`POSTGRES_PASSWORD` is interpolated directly into the connection URLs of `migrate` and axonhub (`postgres://iqlater:<password>@postgres:5432/...`). Characters such as `@`, `:`, `/`, `?`, `#` and `%` break the URL unless escaped. Generate the password with hex characters:

```bash
openssl rand -hex 24
```

## 7. Atlas version and community edition features

- The migration image is pinned to `arigaio/atlas:1.4.0-community` (the open-source build) in `deploy/iqlater/migrate/Dockerfile`, and CI uses the same tag. `latest-community` is a canary build and is not used. When upgrading, change the Dockerfile and `.github/workflows/iqlater-migrations.yml` together.
- `migrate lint` works in this community image without logging in. CI uses it to check the latest migration file.

## 8. CI

`.github/workflows/iqlater-migrations.yml` runs on pull requests targeting `main` and on pushes to `main` (with a PostgreSQL 18 service). It performs, in order:

1. `make migrate-check`: the schema is captured in the migrations and `atlas.sum` is valid.
2. `TestVersionedMigrationsMatchAutoMigrate`: applies all versioned migrations to an empty database and compares the result with the schema produced by `Schema.Create` in `auto` mode. The difference must be empty.
3. `atlas migrate lint --latest 1`: checks the latest migration for destructive changes.

Watch the result the first time this workflow runs on GitHub.
