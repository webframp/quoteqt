#!/bin/bash
# Daily backup script for quotes database
# Keeps compressed backups for 30 days

set -euo pipefail

BACKUP_DIR="/home/exedev/backups"
DB_PATH="/home/exedev/quotes/db.sqlite3"
DATE=$(date +%Y%m%d)
BACKUP_FILE="$BACKUP_DIR/quotes-$DATE.db.gz"
RETENTION_DAYS=30

# Create backup directory if needed
mkdir -p "$BACKUP_DIR"

# Create backup using SQLite's .backup (safe for running DB)
# Then compress it
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/quotes-$DATE.db'"
gzip -f "$BACKUP_DIR/quotes-$DATE.db"

echo "Backup created: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"

# Remove backups older than retention period
find "$BACKUP_DIR" -name "quotes-*.db.gz" -mtime +$RETENTION_DAYS -delete

# List current backups
echo "Current backups:"
ls -lh "$BACKUP_DIR"/quotes-*.db.gz 2>/dev/null || echo "  (none)"
