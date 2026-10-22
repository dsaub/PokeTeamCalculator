# Documentación técnica

Documentación del monorepo **PokeTeamCalculator** (`api/`, `worker/`, `web/`).
Todo lo técnico vive aquí; cada componente conserva un `README.md`/`CLAUDE.md`
corto que apunta a esta carpeta.

## Índice

| Documento | Contenido |
|---|---|
| [analisis-unificacion.md](analisis-unificacion.md) | Hallazgos y correcciones de la unificación de los tres proyectos. |
| [arquitectura.md](arquitectura.md) | Componentes, puertos, contratos y flujos entre API, worker y web. |
| [cola-redis.md](cola-redis.md) | Contrato de la cola en Redis: claves, scripts, estados y recuperación. |
| [despliegue.md](despliegue.md) | Variables de entorno, Docker Compose, CI y puesta en marcha local. |
| [migraciones.md](migraciones.md) | Orden de las migraciones MariaDB y cuándo aplicar cada una. |
| [pruebas.md](pruebas.md) | Cómo ejecutar las suites, con y sin Redis/MariaDB, y sus dobles. |
| [api/catalogo.md](api/catalogo.md) | Endpoints de catálogo (`/api/juegos`, líderes, Pokémon). |
| [api/simulaciones.md](api/simulaciones.md) | Simulaciones: creación, SSE, salud y lado de la API de la cola. |
| [worker.md](worker.md) | Worker Bun: simulación con Pokémon Showdown, cola en Redis y operación. |
| [web/spec.md](web/spec.md) | Especificación de producto del frontend. |
| [web/api-pendiente.md](web/api-pendiente.md) | Historial de carencias detectadas en la API antigua. |
| [web/architecture/index.md](web/architecture/index.md) | Registro de arquitectura del frontend. |
| [web/reference-configs/](web/reference-configs/) | Reglas de trabajo y flujo de desarrollo del frontend. |

## Reparto de responsabilidades

```text
web (React + Vite)  ──HTTP /api──▶  api (Go + Gin)  ──SQL──▶  MariaDB
                                       │  ▲
                            encolar,   │  │  sim:estados
                            cancelar   ▼  │
                                      Redis  ◀──BLMOVE / scripts──  worker (Bun)
```

- **api/**: sirve el catálogo y las simulaciones; es la única que toca MariaDB.
  Nunca simula.
- **worker/**: toma los trabajos de Redis y simula; no sirve HTTP ni conoce
  MariaDB.
- **web/**: solo habla con `/api` (mismo origen o proxy).
- **migraciones/**: esquema MariaDB (solo lo usa la API).
