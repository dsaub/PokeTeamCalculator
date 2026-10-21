# Plan: Simulation frontend against the queue API

> **Status**: Executing
> **Created**: 2026-10-21
> **Slug**: simulation-frontend
> **Artifact Level**: work-package
> **Spec**: `../docs/web/spec.md`

## Goal

Replace the type-chart prototype with the real product flow: pick game and leader, pick the
rival, build a team, send it to `POST /api/simulaciones`, follow the queue over SSE and show
the recommendation. The API contract is fixed by the API agent; it is mirrored in
`src/api/tipos.ts` and must not be changed unilaterally.

## Decisions (confirmed with the user 2026-10-21)

| Topic | Decision |
|-------|----------|
| Language | Migrate to TypeScript (`typescript` dev dependency, `tsc --noEmit` in `check`). |
| Prototype | Remove Pokédex, `/estado`, badges, type-chart heuristic, hardcoded Kalos data and GitHub sprites. |
| Unit tests | `bun test` (+ `@types/bun` for types only). Logic lives in pure modules. |
| E2E | `@playwright/test` against the dev server in mock mode. |
| Routing | react-router: `/` (setup) and `/simulacion/:id` (wait + result). |
| Mock | Vite plugin (`mock/`), enabled only by `MOCK=1`; in-memory queue with real SSE; scenarios forced with the `mock_escenario` cookie, set from a dev-only panel. |

## File layout

| Path | Responsibility |
|------|----------------|
| `src/api/tipos.ts` | Contract types (single source of truth for shapes). |
| `src/api/cliente.ts` | `fetch` wrapper, `ApiError` (status + Spanish message + optional `id`). |
| `src/api/catalogo.ts` | Catalog endpoints with in-memory session cache. |
| `src/api/simulaciones.ts` | POST/GET/DELETE and EventSource factory. |
| `src/dominio/peticion.ts` | Builds the POST body from the form state (gen rules, omit empty). |
| `src/dominio/validacion.ts` | Per-field form validation. |
| `src/dominio/equipoGuardado.ts` | Per-game team persistence in `localStorage`, failure tolerant. |
| `src/dominio/maquina.ts` | Pure reducer for the simulation lifecycle. |
| `src/dominio/seguimiento.ts` | SSE follower with reconnect detection and fallback to 3 s polling. |
| `src/dominio/textos.ts` | Human Spanish text: ETA, recommendation sentence, tie explanation. |
| `src/pages/`, `src/components/` | UI. |
| `mock/` | Dev-only mock API (catalog fixtures + queue). |
| `e2e/` | Playwright tests. |

## Risks

| Risk | Mitigation |
|------|------------|
| Vite proxy buffering SSE | Verify with `curl -N` through the proxy against a test SSE server. |
| Contract gaps (items catalog, duplicate species in results, shared-link context) | Reported to the user; UI degrades gracefully, no contract changes. |

## Verification

`bun run check` = `tsc --noEmit` + `oxlint` + `bun test`; `bun run e2e`; `bun run build`.
