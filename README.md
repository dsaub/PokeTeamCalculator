# PokeTeamCalculator

Simulador de combates de Pokémon contra líderes de gimnasio. Monorepo con tres
componentes, una base MariaDB (solo la usa la API) y una cola en Redis entre la
API y el worker.

```text
web/          React + Vite (interfaz)
api/          Go + Gin (catálogo y cola de simulaciones)
worker/       Bun + Pokémon Showdown (simulador)
migraciones/  Esquema MariaDB
docs/         Documentación técnica
```

## Puesta en marcha rápida

```sh
cp .env.example .env          # ajusta los secretos
docker compose up --build
```

- Web: http://localhost (o `WEB_PORT`)
- API: http://localhost:8080/api/salud
- MariaDB: localhost:3306
- Redis: localhost:6379 (contraseña en `REDIS_PASSWORD`)

La base necesita los datos de **pokedata** para que el catálogo funcione. Las
migraciones `001` y `002` se aplican solas; la `003` se aplica cuando pokedata
ya está importado (ver [docs/migraciones.md](docs/migraciones.md)).

## Desarrollo local

Requisitos: Go 1.27, Bun 1.4, MariaDB 10.6+ y Redis 6.2+ (o
`docker compose up -d db redis`). La API y el worker necesitan la misma
`REDIS_URL`.

```sh
# API
cd api && mise install && go run ./main.go

# Worker
cd worker && bun install && bun src/index.ts

# Web
cd web && bun install && bun run dev        # proxy /api -> 127.0.0.1:8080
cd web && bun run dev:mock                  # API simulada, sin base de datos
```

## Verificación

```sh
cd api && go test ./...
cd worker && bun test && bunx tsc --noEmit    # REDIS_URL_TEST=redis://127.0.0.1:6379 para los de la cola
cd web && bun run check && bun run build
```

## Documentación

Empieza por [docs/README.md](docs/README.md): arquitectura, cola en Redis,
despliegue, migraciones, contrato de la API y especificación del frontend.
