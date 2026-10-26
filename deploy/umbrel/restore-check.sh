#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
backup="${1:?Usage: restore-check.sh /absolute/path/to/snapshot}"
(cd "$backup" && sha256sum -c SHA256SUMS)
db="viki_restorecheck_$(date +%s)_$$"
cleanup() { docker compose exec -T database dropdb -U viki_vozokany --if-exists "$db" >&2; }
trap cleanup EXIT
docker compose exec -T database createdb -U viki_vozokany "$db"
docker compose exec -T database pg_restore -U viki_vozokany --no-owner --exit-on-error -d "$db" < "$backup/database.dump"
docker compose exec -T database psql -U viki_vozokany -d "$db" -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM pages) AS pages, (SELECT count(*) FROM revisions) AS revisions, (SELECT count(*) FROM schema_migrations) AS migrations;"
if [ "$(docker compose exec -T --interactive=false database psql -U viki_vozokany -d "$db" -Atc "SELECT to_regclass('public.repository_snapshots') IS NOT NULL")" = t ]; then
  docker compose exec -T --interactive=false database psql -U viki_vozokany -d "$db" -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM repository_snapshots) AS snapshots, (SELECT count(*) FROM repository_pages WHERE NOT archived) AS current_specs, (SELECT count(*) FROM repository_revisions) AS business_versions;"
fi
tar -tzf "$backup/hermes.tar.gz" >/dev/null
printf 'Restored into disposable database %s; cleanup follows.\n' "$db"
