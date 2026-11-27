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
