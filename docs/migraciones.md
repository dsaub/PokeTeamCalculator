# Migraciones MariaDB

Todas las migraciones viven en `migraciones/` en la raíz del repositorio y se
aplican **en orden lexicográfico**.

| Archivo | Qué hace | Requisitos |
|---|---|---|
| `001_simulaciones.sql` | Crea la tabla `simulaciones`. | MariaDB 10.6+. |
| `002_indices_simulaciones.sql` | Añade `idx_usuario_estado` e `idx_hash_estado`. | Haber aplicado `001`. |
| `003_v_gym_teams_formas.sql` | Recrea `v_gym_teams` para conservar las formas regionales. | pokedata importado (`v_gym_battles`, `gym_battle_pokemon`, `pokemon`…). |

## Aplicar a mano

```sh
mariadb -u usuario -p poketeam < migraciones/001_simulaciones.sql
mariadb -u usuario -p poketeam < migraciones/002_indices_simulaciones.sql
# Solo si la base ya tiene los datos de pokedata:
mariadb -u usuario -p poketeam < migraciones/003_v_gym_teams_formas.sql
```

`001` y `002` son idempotentes (`IF NOT EXISTS`). `003` usa
`CREATE OR REPLACE VIEW`, así que también se puede reaplicar.

## Docker Compose

El servicio `migrator` aplica `001` y `002` y, si detecta la vista
`v_gym_battles`, también `003`. Si la base se crea vacía, `003` se omite: la API
importa pokedata al arrancar y la aplica ella misma (ver
[despliegue.md](despliegue.md#datos-de-pokedata)). Si importas pokedata por otra
vía, reaplícala:

```sh
docker compose run --rm migrator
```

## Reglas

- No editar a mano las migraciones ya aplicadas en producción; añadir una nueva
  con el siguiente número.
- pokedata recrea sus vistas al importar: **reaplicar `003`** después de cada
  importación hecha fuera de la API.
- Solo la API usa la tabla `simulaciones`: el worker no se conecta a MariaDB.
  La cola vive en Redis (ver [cola-redis.md](cola-redis.md)); la tabla es la
  fuente de verdad desde la que la API la reconstruye.
- La columna `latido` ya no se usa (los latidos están en Redis) y `worker` la
  rellena la API con el id del worker que tomó el trabajo.
