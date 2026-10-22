# Pruebas

Cada componente tiene su suite. Casi todo corre sin servicios externos; lo que
necesita Redis o MariaDB se salta si no se indica dónde están.

## Sin servicios

```sh
cd api && go test ./...                  # miniredis y go-sqlmock, sin MariaDB
cd worker && bun test && bunx tsc --noEmit
cd web && bun run check                  # tsc + oxlint + bun test
```

## Con Redis y MariaDB

```sh
docker compose up -d --wait redis db

# Base aparte: los tests de MySQL crean y vacían la tabla `simulaciones`.
docker compose exec -T db mariadb -uroot -p"$DB_ROOT_PASSWORD" \
  -e "CREATE DATABASE IF NOT EXISTS pruebas; GRANT ALL ON pruebas.* TO 'pokedata'@'%';"

cd api && DATABASE_DSN_TEST='pokedata:<clave>@tcp(127.0.0.1:3306)/pruebas?parseTime=true' go test ./...
cd worker && REDIS_URL_TEST='redis://:<clave>@127.0.0.1:6379/15' bun test
```

- Nunca apuntes `DATABASE_DSN_TEST` a `pokedata`: los tests vacían la tabla.
- `TestMySQLCatalogoHabilidadValida` y los `*_mysql_test.go` del catálogo
  necesitan pokedata importado en esa base; si no está, se saltan.
- `test/cola.test.ts` usa un prefijo de claves aleatorio y lo borra al terminar.
  `test/herramientas.test.ts` encola con el prefijo real `sim:` un id muy alto y
  borra solo lo que crea.

## Cobertura

```sh
cd api && go test -coverprofile=cover.out ./... && go tool cover -func=cover.out
cd worker && bun test --coverage
cd web && bun test --coverage
```

`api/queries/` es código generado por sqlc y `main.go` solo cablea: no cuentan
para la cobertura útil.

## Cómo están montadas

### API (`api/`)

- `endpoints/simulaciones/harness_test.go`: `nuevaAPI` monta la API entera con
  `memRepo` (MariaDB en memoria, `fakes_test.go`) y miniredis. `memRepo.fallarEn`
  simula el fallo de un método concreto.
- Los repositorios MySQL se prueban con go-sqlmock (forma de las filas y
  errores) y, con `DATABASE_DSN_TEST`, contra MariaDB (consultas reales).
- `main.go` expone `nuevoRouter` para probar las rutas, el 404 y el 405.

### Worker (`worker/`)

- `test/fixtures/dobles.ts`: `ColaFalsa` y `PoolFalso` para probar el
  `Procesador` y el arranque sin Redis ni hilos.
- `test/fixtures/hilo-falso.ts`: hilo de pruebas para el pool (colgarse,
  romperse, fallar, salir).
- `src/arranque.ts` contiene el arranque y el apagado; `src/index.ts` solo
  registra las señales.
- Con Redis inalcanzable el cliente de Bun reintenta unos 30 s antes de
  rendirse: los tests de ese caso usan dobles, no un Redis real.

### Web (`web/`)

- `bunfig.toml` precarga `test/dom.ts`: registra happy-dom, conserva el `fetch`
  de Bun y limpia el DOM, `localStorage` y los dobles tras cada test.
- `test/fetch.ts`: `rutas({ 'GET /juegos': [...] })` sustituye `fetch` por
  rutas y `falsearEventSource()` sustituye el SSE.
- Las páginas se prueban con Testing Library dentro de un `MemoryRouter`.
- `mock/servidor.test.ts` monta el plugin del mock en un servidor HTTP real.
