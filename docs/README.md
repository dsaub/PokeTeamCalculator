# Documentación técnica

Documentación del monorepo **PokeTeamCalculator** (`api/`, `worker/`, `web/`).
Todo lo técnico vive aquí; cada componente conserva un `README.md`/`CLAUDE.md`
corto que apunta a esta carpeta.

## Índice

| Documento | Contenido |
|---|---|
| [analisis-unificacion.md](analisis-unificacion.md) | Hallazgos y correcciones de la unificación de los tres proyectos. |
| [arquitectura.md](arquitectura.md) | Componentes, puertos, contratos y flujos entre API, worker y web. |
| [despliegue.md](despliegue.md) | Variables de entorno, Docker Compose, CI y puesta en marcha local. |
| [migraciones.md](migraciones.md) | Orden de las migraciones MariaDB y cuándo aplicar cada una. |
| [api/catalogo.md](api/catalogo.md) | Endpoints de catálogo (`/api/juegos`, líderes, Pokémon). |
| [api/simulaciones.md](api/simulaciones.md) | Cola de simulaciones, SSE, salud y contrato con el worker. |
| [worker.md](worker.md) | Worker Bun: simulación con Pokémon Showdown, cola y operación. |
| [web/spec.md](web/spec.md) | Especificación de producto del frontend. |
| [web/api-pendiente.md](web/api-pendiente.md) | Historial de carencias detectadas en la API antigua. |
| [web/architecture/index.md](web/architecture/index.md) | Registro de arquitectura del frontend. |
| [web/reference-configs/](web/reference-configs/) | Reglas de trabajo y flujo de desarrollo del frontend. |

## Reparto de responsabilidades

```text
web (React + Vite)  ──HTTP /api──▶  api (Go + Gin)  ──INSERT/SELECT──▶  MariaDB
                                         │  ▲                            ▲
                                         │  └── callback interno ──┐    │
                                         └── POST /despertar ──▶ worker (Bun) ──┘
```

- **api/**: sirve el catálogo y la cola; nunca simula.
- **worker/**: consume la cola y simula; no sirve tráfico público.
- **web/**: solo habla con `/api` (mismo origen o proxy).
- **migraciones/**: esquema MariaDB compartido por API y worker.
