# Despliegue

## Variables de entorno

El `.env.example` de la raíz es la referencia única. Copia a `.env` y cambia
todos los secretos.

### API (Go)

| Variable | Obligatoria | Por defecto | Uso |
|---|---|---|---|
| `PORT` | no | `8080` | Puerto HTTP público. |
| `DB_HOST` / `DB_PORT` | no | `127.0.0.1` / `3306` | MariaDB. En Compose, `db`. |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | no | `pokedata` / vacío / `pokedata` | Credenciales. |
| `DB_TLS` | no | `preferred` | `preferred`, `true`, `skip-verify` o `false`. |
| `REDIS_URL` | sí | | `redis://:clave@host:6379/0`. En Compose la arma con `REDIS_PASSWORD`. |
| `SECRETO_COOKIE` | sí (32+ bytes) | | Firma de la cookie `sid`. |
| `LATIDO_CADUCADO_WORKER` | no | `15s` | Un worker cuenta como disponible si su último latido es más reciente. |
| `PROXIES_CONFIABLES` | no | vacío | IPs/CIDR que pueden enviar `X-Forwarded-For`. |

El resto (`SIMULACIONES`, `MAX_COLA`, `CACHE_TTL`, `LIMITE_POR_MINUTO`…) está
documentado en [api/simulaciones.md](api/simulaciones.md).

### Worker (Bun)

| Variable | Obligatoria | Por defecto | Uso |
|---|---|---|---|
| `REDIS_URL` | sí | | La misma que la API. |
| `HILOS` | no | núcleos − 1 | Hilos de simulación. |
| `TRABAJOS_SIMULTANEOS` | no | `2` | Trabajos en paralelo. |

En Compose el worker no lee el `.env` entero: recibe solo `REDIS_URL` y sus
propias variables, así que no tiene las credenciales de MariaDB.

### Redis

| Variable | Obligatoria | Por defecto | Uso |
|---|---|---|---|
| `REDIS_PASSWORD` | no | `change-me-redis` | Contraseña del contenedor `redis`. Va dentro de `REDIS_URL`: sin `@`, `:`, `/` ni `?`. |

El resto está en [worker.md](worker.md).

## Docker Compose

`docker-compose.yml` levanta:

| Servicio | Imagen / build | Puerto |
|---|---|---|
| `db` | `mariadb:11.4` | 3306 (solo `127.0.0.1`) |
| `redis` | `redis:8.10-alpine`, con AOF y contraseña | 6379 (solo `127.0.0.1`) |
| `migrator` | `mariadb:11.4` + `migraciones/aplicar.sh` | — |
| `api` | `Dockerfile` (raíz, contexto `api/`) | 8080 |
| `worker` | `worker/Dockerfile` | — |
| `web` | `web/Dockerfile` + nginx | `${WEB_PORT:-80}` |

```sh
cp .env.example .env
docker compose up --build
```

En Compose, la API confía en la red `172.16.0.0/12` para leer
`X-Forwarded-For` de nginx. Si cambias la red de Docker, ajusta
`PROXIES_CONFIABLES`.

El `migrator` aplica `001` y `002` siempre. La `003` solo si detecta las vistas
de pokedata; si importas pokedata después:

```sh
docker compose run --rm migrator
```

## Desarrollo sin Docker

1. MariaDB 10.6+ con la base y el usuario de `.env`, y Redis 6.2+ (o
   `docker compose up -d db redis`).
2. `cd api && go run ./main.go` (con las variables exportadas, incluida
   `REDIS_URL=redis://:change-me-redis@127.0.0.1:6379/0`)
3. `cd worker && bun src/index.ts` (con la misma `REDIS_URL` en `worker/.env` o
   exportada)
4. `cd web && bun run dev`

### Actualizar desde la cola en MariaDB

No mezcles el worker antiguo (que lee MariaDB) con la API nueva: procesarían los
mismos trabajos dos veces. Para el worker antiguo (con `SIGTERM` devuelve lo que
tenga a `pendiente`), arranca la API nueva (la reconciliación pasa a Redis los
pendientes de MariaDB en orden de creación) y después el worker nuevo.
`docker compose up --build` lo hace todo de una vez.

## CI

- `.github/workflows/docker-trivy.yml`: tests de Go, worker y web; build de la
  imagen de la API y escaneo con Trivy. El job `checks` levanta un Redis para
  los tests de la cola del worker (`REDIS_URL_TEST`); los de Go usan miniredis.
- `Jenkinsfile` (raíz, `agent none`): análisis de seguridad **en paralelo** por
  componente en `srv01` (SonarQube con `api/`, `worker/` y
  `web/sonar-project.properties`, Hadolint para los tres Dockerfiles y Gitleaks
  global). Después crea un sandbox x86_64 de Daytona (`prepareDaytona`,
  credencial *Secret text* `daytona-token`) y construye las tres imágenes **una tras otra** con `buildMultiArchImage`;
  dentro de cada una, `linux/arm64` en `srv01` y `linux/amd64` en el sandbox
  van en paralelo, sin emulación QEMU. Cada imagen se publica en
  GHCR (`:latest` y `:<commit>`, credencial `github-pat`).
- El snapshot `mlib-dind` lleva los recursos del sandbox (Daytona no admite
  `--cpu/--memory/--disk` al crear desde un snapshot). Las imágenes amd64 se
  compilan de una en una; conviene al menos 4 CPU y 8 GB (el disco tiene un máximo de 10 GB por sandbox):
  `daytona snapshot create mlib-dind --image docker:28.3.3-dind --cpu 4 --memory 8 --disk 10`.
- El cortafuegos de salida del sandbox (lista blanca de Daytona) bloquea todos
  los mirrors de Alpine (`dl-cdn.alpinelinux.org` y demás): un `apk` dentro de
  un build falla con `TLS: unspecified error`. Sí deja pasar `deb.debian.org`,
  `archive.ubuntu.com`, npm, GitHub y `proxy.golang.org`. Por eso las imágenes
  del worker (`oven/bun:*-slim`) y de la web (`nginx:*`) son Debian y se
  parchean con `apt-get upgrade`. Una imagen Alpine solo vale si no instala
  nada (como el builder de la API).
- Trivy escanea después las imágenes publicadas, en paralelo y leyéndolas del
  registro. **Falla el pipeline** si encuentra cualquier vulnerabilidad con
  corrección disponible (todas las severidades); las que aún no tienen parche se
  ignoran con `--ignore-unfixed`. Como el push va antes, una imagen vulnerable
  ya está en GHCR cuando el pipeline falla.
- El `post { always }` borra **siempre** el sandbox de Daytona, falle o no el
  build. Cada agente limpia lo suyo: `buildMultiArchImage` ejecuta
  `cleanDocker()`, y la stage de Trivy borra su volumen de caché y hace
  `cleanWs()`.

## Notas de seguridad

- La API y el worker no se hablan por HTTP: no hay puertos internos que
  proteger. El worker no escucha en ningún puerto.
- Redis solo se publica en `127.0.0.1`; nunca expongas `6379` fuera del host.
- El worker no tiene credenciales de MariaDB: si se compromete, solo alcanza
  Redis.
- Cambia `REDIS_PASSWORD`, `SECRETO_COOKIE`, `DB_PASSWORD` y
  `DB_ROOT_PASSWORD` antes de desplegar.
