#!/bin/sh
# Aplica las migraciones en orden. 003 necesita los datos de pokedata; si aún
# no están, se omite y puede reaplicarse después con:
#   docker compose run --rm migrator
set -eu

: "${DB_HOST:=db}"
: "${DB_PORT:=3306}"
: "${DB_NAME:=pokedata}"
: "${DB_ROOT_PASSWORD:?DB_ROOT_PASSWORD es obligatoria}"

sql() {
  mariadb --protocol=tcp -h "$DB_HOST" -P "$DB_PORT" -u root -p"$DB_ROOT_PASSWORD" "$DB_NAME" "$@"
}

echo "Esperando a MariaDB en $DB_HOST:$DB_PORT…"
intentos=0
until mariadb-admin --protocol=tcp -h "$DB_HOST" -P "$DB_PORT" -u root -p"$DB_ROOT_PASSWORD" ping --silent >/dev/null 2>&1; do
  intentos=$((intentos + 1))
  if [ "$intentos" -ge 30 ]; then
    echo "MariaDB no respondió en 60 s." >&2
    exit 1
  fi
  sleep 2
done

for archivo in /migraciones/001_simulaciones.sql /migraciones/002_indices_simulaciones.sql; do
  echo "Aplicando $(basename "$archivo")…"
  sql < "$archivo"
done

if sql -N -e "SELECT COUNT(*) FROM information_schema.views WHERE table_schema = DATABASE() AND table_name = 'v_gym_battles'" | grep -qx '1'; then
  echo "pokedata detectado: aplicando 003_v_gym_teams_formas.sql…"
  sql < /migraciones/003_v_gym_teams_formas.sql
else
  echo "pokedata todavía no está importado: se omite 003. Reaplícala después de importarlo."
fi

echo "Migraciones terminadas."
