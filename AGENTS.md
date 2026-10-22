# PokeTeamCalculator — contexto para agentes

Monorepo con tres componentes desplegables, una base MariaDB (solo la usa la
API) y una cola en Redis entre la API y el worker. La documentación técnica vive
en `docs/` (índice en `docs/README.md`); no la dupliques dentro de los
componentes.

## Límites y puertos

- `api/` — Go 1.27 + Gin: catálogo y cola. Nunca simula.
- `worker/` — Bun 1.4.3 + Pokémon Showdown: toma los trabajos de Redis y
  simula. No sirve HTTP ni se conecta a MariaDB.
- `web/` — React 19 + Vite 8: solo habla con `/api` (proxy en desarrollo).
- `migraciones/` — esquema MariaDB (solo lo usa la API).
- Puertos: API `8080`, MariaDB `3306`, Redis `6379`. La API y el worker no se
  hablan por HTTP: todo pasa por Redis (`docs/cola-redis.md`).
- Docker: la imagen de la API se construye desde la raíz (el `Dockerfile` copia
  `api/`); worker y web desde sus propios directorios.

## Comandos

```sh
cd api && go test ./...
cd api && go test ./endpoints/simulaciones -run TestNombre   # un solo test
cd api && golangci-lint run

cd worker && bun test && bunx tsc --noEmit
cd worker && bun test test/simular.test.ts
cd worker && REDIS_URL_TEST=redis://127.0.0.1:6379 bun test test/cola.test.ts

cd web && bun run check      # tsc + oxlint + bun test
cd web && bun test src/dominio/peticion.test.ts
cd web && bun run build      # check no compila; el CI sí construye
```

- Usa `mise install` en cada componente (Go 1.27.2, Bun 1.4.3). Bun, no
  Node/npm: `bun install`, `bun test`, `bunx tsc`.
- La API Go **no** carga `.env`: exporta las variables o usa Compose. Bun sí
  carga el `.env` del directorio actual (`worker/.env`).
- Sin `REDIS_URL` y `SECRETO_COOKIE` (32+) la API no arranca; el worker solo
  exige `REDIS_URL` (la misma que la API).
- `cd web && bun run dev:mock` levanta una API en memoria, sin MariaDB, para
  probar el flujo completo del frontend.
- Simulador sin base de datos: `cd worker && bun herramientas/cli.ts
  herramientas/ejemplo.json` (`--json`, `--textos`).

## Tests que se saltan

- API: los `*_mysql_test.go` se omiten sin `DATABASE_DSN_TEST`.
- Worker: `test/cola.test.ts` se omite sin `REDIS_URL_TEST`. Usa un prefijo de
  claves aleatorio y lo borra, así que no pisa datos. El CI levanta Redis para
  estos tests; los de Go usan miniredis y corren siempre.
- El CI no levanta MariaDB: esos tests no corren allí. No los des por fallidos.
- No hay suite E2E/Playwright aunque `@playwright/test` esté en devDependencies.

## Contratos (cambiar los tres a la vez)

- `api/endpoints/simulaciones/peticion.go`, `worker/src/sim/tipos.ts` y
  `web/src/api/tipos.ts`.
- Las claves JSON son en español (`especie`, `movimientos`, `rival`,
  `candidatos`); los valores de especies, movimientos, habilidades, objetos y
  naturalezas van en inglés de Showdown, con sufijo regional (`Yamask-Galar`).
- El navegador no envía `simulaciones` ni `opciones`: los fija la API. No los
  añadas a `PeticionCliente`.
- En `Stats` los campos son punteros a propósito: un IV/EV de 0 es válido y
  distinto de «no indicado».
- La cola en Redis es otro contrato: claves, campos del hash y del stream
  `sim:estados` y scripts. Cambia a la vez `docs/cola-redis.md`,
  `api/endpoints/simulaciones/cola.go` y `worker/src/cola.ts`.
- Antes de tocar la cola o el SSE, lee `docs/cola-redis.md`,
  `docs/api/simulaciones.md` y `docs/worker.md`.

## Datos, esquema y codegen

- `migraciones/` es la fuente del esquema de `simulaciones`; se aplica en orden
  lexicográfico. No edites migraciones ya aplicadas: añade la siguiente.
- `api/schema.sql` se genera desde pokedata y `api/queries/` desde sqlc. No los
  edites a mano: cambia `api/query.sql` y ejecuta `sqlc generate`. La tabla
  `simulaciones` no pasa por sqlc.
- `003_v_gym_teams_formas.sql` se omite si pokedata no está importado y pokedata
  la pisa en cada importación: reaplícala con
  `docker compose run --rm migrator`. Con la base vacía el catálogo devuelve
  `500`; es el comportamiento esperado.

## Reglas locales

- `web/AGENTS.md` (duplicado en `web/CLAUDE.md`) manda en el frontend; el estado
  y los planes viven en `web/tasks/` y `web/plans/`. Sus comandos
  `repo-harness` solo funcionan si esa herramienta está instalada.
- `worker/CLAUDE.md` es guía genérica de Bun y aplica solo a `worker/`; el
  frontend usa Vite a propósito.
- Documentación y mensajes de usuario en español.
