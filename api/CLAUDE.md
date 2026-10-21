# api — Go + Gin

API REST que sirve el catálogo de pokedata y gestiona la cola de simulaciones.
La documentación completa está en [`../docs/`](../docs/).

## Comandos

```sh
mise install
go run ./main.go
go build ./...
go test ./...
golangci-lint run
sqlc generate
```

La configuración se lee de variables de entorno (ver `.env.example` en la raíz).
`LoadConfig` exige `TOKEN_INTERNO` (16+ caracteres) y `SECRETO_COOKIE` (32+
bytes). El servidor interno escucha en `HOST_INTERNO:PUERTO_INTERNO`
(por defecto `127.0.0.1:8081`).

## Estructura

- `main.go` — arranque, rutas y apagado limpio.
- `database/` — conexión MySQL/MariaDB y estado de la base.
- `endpoints/catalogo/` — juegos, líderes y Pokémon (solo lectura).
- `endpoints/simulaciones/` — cola, SSE, rate limit, salud y callback interno.
- `endpoints/pokemons/` — endpoints antiguos `/api/v1/pokemon`.
- `queries/` — generado por sqlc; no editar a mano.
- `schema.sql` y `query.sql` — fuente de sqlc.

Las migraciones **no** viven en este directorio: están en
[`../migraciones/`](../migraciones/).

## Contratos

- [Catálogo](../docs/api/catalogo.md)
- [Simulaciones y worker](../docs/api/simulaciones.md)
- [Arquitectura](../docs/arquitectura.md)
