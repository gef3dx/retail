#!/bin/sh
# Бэкап PostgreSQL: pg_dump в ./backups с ротацией (по умолчанию держать 7 штук).
# Использование: ./scripts/backup.sh
# Cron (ежедневно в 03:00): 0 3 * * * cd /opt/retail && ./scripts/backup.sh >> backups/cron.log 2>&1
set -eu

BACKUP_DIR="${BACKUP_DIR:-./backups}"
KEEP="${BACKUP_KEEP:-7}"
CONTAINER="${PG_CONTAINER:-retail-pg}"
DB="${POSTGRES_DB:-retail}"
USER="${POSTGRES_USER:-retail}"

mkdir -p "$BACKUP_DIR"
TS=$(date +%Y%m%d-%H%M%S)
FILE="$BACKUP_DIR/retail-$TS.sql.gz"

if docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  docker exec "$CONTAINER" pg_dump -U "$USER" -d "$DB" | gzip > "$FILE"
else
  # Локальный postgres без docker.
  pg_dump -h "${POSTGRES_HOST:-localhost}" -p "${POSTGRES_PORT:-5432}" -U "$USER" -d "$DB" | gzip > "$FILE"
fi

echo "backup written: $FILE"
ls -1t "$BACKUP_DIR"/retail-*.sql.gz | tail -n +$((KEEP + 1)) | xargs -r rm -f
echo "rotation: keeping last $KEEP"

# Проверка восстановления (dry-run в tmp-БД, только если задан RESTORE_CHECK=1):
if [ "${RESTORE_CHECK:-0}" = "1" ]; then
  TMPDB="restore_check_$TS"
  docker exec "$CONTAINER" psql -U "$USER" -d postgres -c "CREATE DATABASE $TMPDB" >/dev/null
  gunzip -c "$FILE" | docker exec -i "$CONTAINER" psql -U "$USER" -d "$TMPDB" -q -f - >/dev/null
  docker exec "$CONTAINER" psql -U "$USER" -d postgres -c "DROP DATABASE $TMPDB" >/dev/null
  echo "restore check: OK"
fi
