# Análisis de la unificación

> Fecha: 21/10/2026. Alcance: `api/`, `worker/`, `web/`, despliegue y
> migraciones. Este documento resume los problemas encontrados al unificar los
> tres proyectos y lo que se corrigió.

## Resumen ejecutivo

| Componente | Estado inicial | Estado tras la revisión |
|---|---|---|
| `api/` | Compila y pasa tests; sin `mise.toml`; paginación y errores con defectos; servidor interno fijo en `127.0.0.1`; caché de errores. | Compila, tests y `golangci-lint` en verde; paginación/errores corregidos; `HOST_INTERNO`; caché solo en respuestas OK. |
| `worker/` | Tests y typecheck en verde; migración fuera de raíz; un worker podía sobrescribir un trabajo recuperado por otro; combate sin `destroy()` en errores. | Tests/typecheck en verde; guardas por `worker`; `destroy()` en `finally`; migración compartida. |
| `web/` | **No compilaba**: faltaban `main.tsx`, `App`, páginas, `vite.config.ts` y la API mock; tipos del contrato desalineados. | Compila, `tsc`, `oxlint`, tests y build en verde; flujo completo juego → líder → equipo → simulación → resultado; mock de desarrollo. |
| Raíz | Sin documentación central, migraciones dispersas, CI/Docker apuntando al proyecto Go antiguo, `.env` incompleto. | `docs/`, `migraciones/`, Dockerfile/Jenkins raíz, Compose con MariaDB/API/worker/web, CI con checks. |

## Hallazgos por componente

### API (Go)

1. **Paginación inválida**: `?page=0` y `?page=-1` llegaban a la base; páginas
   fuera de rango devolvían `pokemons: null` y `last: false`; además la consulta
   no tenía `ORDER BY`, así que el orden entre páginas no era estable. Se valida
   `page >= 1`, se devuelve `[]`, `last: true`, `total`, `page_size` y
   `total_pages`, y la consulta ordena por `sort_order, id`.
2. **Error equivocado en `GET /api/v1/pokemon/:id`**: un id no numérico
   respondía `bad_page_entry`. Ahora responde `bad_pokemon_id`.
3. **Errores de base de datos como 404**: cualquier fallo de consulta se
   trataba como «no encontrado». Ahora `sql.ErrNoRows` → 404 y el resto → 500.
4. **Caché de errores**: `Cache-Control` se aplicaba a todo el grupo, incluidos
   400/404/500. Ahora solo a respuestas correctas.
5. **Servidor interno fijo**: `127.0.0.1:8081` impedía el callback en Docker.
   Se añadió `HOST_INTERNO` (por defecto `127.0.0.1`; Compose usa `0.0.0.0`).
6. **Movimientos duplicados**: se detectaban en el worker (trabajo en `error`)
   en vez de en la validación (400). Se validan en la API.
7. **Toolchain**: faltaba `api/mise.toml`; se recuperó Go 1.27.2, gopls,
   golangci-lint y sqlc.
8. **Código muerto**: se eliminó el middleware `Cachear` al dejar de usarse.

### Worker (Bun)

1. **Condición de carrera al completar**: un worker cuyo latido había caducado
   podía sobrescribir el resultado de otro que ya había recuperado el trabajo.
   `completar` y `fallar` ahora exigen `worker = ?`.
2. **Toma doble**: el `UPDATE` de `tomar` no comprobaba `estado = 'pendiente'`;
   se añadió la guarda.
3. **Fuga en combates**: si Showdown lanzaba una excepción a mitad de combate,
   `battle.destroy()` no se ejecutaba. Ahora va en `finally`.
4. **Detección de versión**: MariaDB puede reportar `5.5.5-10.6.x`; el validador
   rechazaba versiones válidas. Ahora quita ese prefijo antes de comparar.
5. **Servidor interno fijo en `127.0.0.1`**: la API no podía llamar al worker
   desde otro contenedor. Se añadió `HOST_WORKER` (Compose usa `0.0.0.0`),
   igual que `HOST_INTERNO` en la API.
6. **Migración fuera de sitio**: `worker/migraciones/001` se movió a
   `migraciones/` y los tests leen la ruta nueva.

### Web (React + Vite)

1. **El proyecto no arrancaba**: `index.html` cargaba `/src/main.tsx`, que no
   existía; tampoco había `App`, páginas ni `vite.config.ts`. Se implementó el
   flujo completo con los módulos de dominio ya probados.
2. **Contrato desalineado**:
   - `Lider.id` era `string`; la API devuelve número.
   - `Lider.tipo` podía ser `null`.
   - `Movimiento.categoria` llegaba en minúsculas (`physical`) y podía ser
     `null`; `potencia` también puede ser `null`.
   - La respuesta 202 de un duplicado puede estar en `procesando`, sin posición.
   - `TypeBadge` esperaba tipos capitalizados y la API los devuelve en
     minúsculas; ahora acepta ambos.
3. **Regla de stats incorrecta**: se ocultaba `spd` en gen 2, pero desde gen 2
   Sp. Atk y Sp. Def son stats separados; solo gen 1 usa un único «Especial».
   Corregido en `camposDeGen` y en sus tests.
4. **Sin entorno de desarrollo aislado**: se añadió `bun run dev:mock`, una API
   en memoria con catálogo, cola y SSE que no necesita MariaDB.
5. **Sin build**: se añadió `vite.config.ts` (proxy `/api` → `127.0.0.1:8080`)
   y se verificó `bun run build`.

### Estructura, despliegue y CI

1. **Migraciones dispersas** en `api/migraciones` y `worker/migraciones`; ahora
   todas en `migraciones/` (001 cola, 002 índices, 003 vista).
2. **Documentación dispersa** en `api/docs` y `web/docs`; ahora en `docs/`.
3. **Compose inexistente para el monorepo**: `api/docker-compose.yaml` solo
   definía la app y apuntaba a un host `db` inexistente. Se creó
   `docker-compose.yml` con MariaDB, migrador, API, worker y web.
4. **Dockerfile/Jenkins raíz obsoletos**: apuntaban al Go antiguo. Ahora la raíz
   construye `api/`; se añadieron `worker/Dockerfile` y `web/Dockerfile`.
5. **CI solo Docker**: se añadió un job de checks (Go, worker, web) antes del
   build y del escaneo Trivy. En Jenkins, SonarQube, Hadolint y Trivy se
   ejecutan en paralelo para los tres componentes. Trivy falla si hay alguna
   vulnerabilidad con corrección disponible (`--ignore-unfixed` + `--exit-code 1`).
   El `post` limpia imágenes construidas, imágenes sin usar de más de 24 h,
   caché de BuildKit, volumen de Trivy y workspace.
6. **`.env.example` incompleto**: no tenía `DATABASE_URL`, `URL_CALLBACK` ni las
   variables del worker. Ahora cubre los tres componentes.
7. **Callback mal documentado**: el worker apuntaba a `8080` (público) en vez de
   `8081` (interno). Corregido en docs y ejemplo de configuración.
8. **`.gitignore` insuficiente**: no ignoraba `node_modules`, `dist`, `_ops`,
   logs ni `.env.*`. Corregido.

## Deuda conocida

- **pokedata**: el catálogo no funciona con una base vacía; hay que importar
  pokedata. La migración `003` se omite si no detecta sus vistas.
- **Objetos y naturalezas**: la API no expone catálogo de objetos/naturalezas;
  el formulario web no los ofrece (documentado en la spec).
- **EVs/IVs**: el formulario MVP no los edita, aunque el contrato y el worker
  los admiten.
- **SSE multi-réplica**: el hub es local al proceso; el sondeo mitiga callbacks
  perdidos, pero no comparte eventos entre réplicas.
- **Tests de integración**: los tests de MariaDB se saltan sin
  `DATABASE_DSN_TEST` / `DATABASE_URL_TEST`; en CI no hay servicio MariaDB.
- **Imágenes de Pokémon**: el catálogo no devuelve sprites; la UI no muestra
  imágenes.
- **Web E2E**: no hay suite Playwright (el plan la preveía); la verificación
  actual es typecheck + lint + tests unitarios + build.

## Verificación ejecutada

```sh
cd api    && go build ./... && go test ./... && go vet ./... && golangci-lint run ./...
cd worker && bun test && bunx tsc --noEmit
cd web    && bun run check && bun run build
docker compose build
docker compose up -d
```

Resultados locales: Go build/tests/vet/lint OK; worker 48 tests + 18 skip
(MariaDB) y typecheck OK; web 60 tests, typecheck, lint y build OK.

### Verificación real con Docker

- `docker compose build`: API, worker y web construidos correctamente.
- `docker compose up -d`: MariaDB healthy; `migrator` aplicó `001` y `002` y
  omitió `003` (pokedata no importado); API, worker y web arrancaron.
- Flujo completo a través de nginx (`http://localhost/api/...`):
  - `POST /api/simulaciones` → `202` y trabajo procesado por el worker.
  - `GET /api/simulaciones/{id}` → `completado` con ranking real de Showdown.
  - SSE (`/eventos`) → `procesando` + `completado`.
  - Misma petición → `200` con `cache: true`.
  - `DELETE` → `204`; segundo `DELETE` → `409`.
  - Ruta inexistente → `404`; método no permitido → `405` JSON.
  - Worker parado → trabajo queda `pendiente`; al arrancarlo se procesa.
- Migración `003` probada contra un esquema mínimo de pokedata: la vista se
  crea y `yamask-galar` sale como `Yamask-Galar`.
- El catálogo (`/api/juegos`, etc.) devuelve `500` con la base vacía, que es el
  comportamiento esperado hasta importar pokedata.
