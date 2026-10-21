# Repo Agent Context

Monorepo **PokeTeamCalculator** con tres componentes independientes:

- `api/` — Go + Gin. Contratos y arquitectura en `docs/api/`.
- `worker/` — Bun + Pokémon Showdown. Documentación en `docs/worker.md`.
- `web/` — React + Vite. Producto en `docs/web/spec.md`.
- `migraciones/` — esquema MariaDB compartido.
- `docs/` — documentación técnica.

## Reglas

- La documentación técnica se escribe en `docs/`, no dentro de cada componente.
- Las migraciones MariaDB van en `migraciones/` y se numeran en orden.
- No edites `api/queries/` (generado por sqlc); cambia `api/query.sql` y ejecuta
  `sqlc generate`.
- Los contratos compartidos están en `api/endpoints/simulaciones/peticion.go`,
  `worker/src/sim/tipos.ts` y `web/src/api/tipos.ts`; cámbialos a la vez.
- Antes de tocar la cola, lee `docs/api/simulaciones.md` y `docs/worker.md`.
- Verifica con `cd api && go test ./...`, `cd worker && bun test`, y
  `cd web && bun run check`.

## Contexto específico

Cada componente puede tener su propio `CLAUDE.md`/`AGENTS.md` con reglas locales;
este archivo es el contrato raíz.
