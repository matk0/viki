#!/bin/bash
set -euo pipefail
umask 077
cd "$(dirname "$0")"
exec 9>backup.lock
flock -n 9 || exit 0
backup_root="$PWD/backups"
mkdir -p "$backup_root"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
partial="$backup_root/.partial-$stamp"
final="$backup_root/$stamp"
mkdir "$partial"
# Stop only this instance so the database and assistant snapshot agree.
resume() { docker compose up -d --wait --wait-timeout 240 >&2; }
trap resume EXIT
docker compose stop sync viki hermes >&2
docker compose exec -T database pg_dump -U viki_vozokany -d viki_vozokany --format=custom > "$partial/database.dump"
hermes_data="$(docker volume inspect --format '{{.Mountpoint}}' viki-vozokany-hermes)"
tar -czf "$partial/hermes.tar.gz" -C "$hermes_data" .
cp .env compose.yaml backup.sh restore-check.sh install-backup-schedule.sh "$partial/"
[ ! -f version.json ] || cp version.json "$partial/"
[ ! -f .sync.env ] || cp .sync.env "$partial/"
(cd "$partial"; files=(database.dump hermes.tar.gz compose.yaml .env backup.sh restore-check.sh install-backup-schedule.sh); [ ! -f version.json ] || files+=(version.json); [ ! -f .sync.env ] || files+=(.sync.env); sha256sum "${files[@]}" > SHA256SUMS)
mv "$partial" "$final"
resume
trap - EXIT
# Retain fourteen daily snapshots; never follow symlinks or cross filesystem boundaries.
find "$backup_root" -xdev -mindepth 1 -maxdepth 1 -type d -name '20??????T??????Z' -mtime +13 -exec rm -rf -- {} +
printf '%s\n' "$final"
