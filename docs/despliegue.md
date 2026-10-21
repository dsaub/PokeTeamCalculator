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
| `TOKEN_INTERNO` | sí (16+) | | Secreto compartido con el worker. |
| `SECRETO_COOKIE` | sí (32+ bytes) | | Firma de la cookie `sid`. |
| `URL_WORKER` | no | `http://127.0.0.1:7070` | Despertar al worker. |
| `PUERTO_INTERNO` | no | `8081` | Callback del worker. |
| `HOST_INTERNO` | no | `127.0.0.1` | En Compose, `0.0.0.0`. |
| `PROXIES_CONFIABLES` | no | vacío | IPs/CIDR que pueden enviar `X-Forwarded-For`. |

El resto (`SIMULACIONES`, `MAX_COLA`, `CACHE_TTL`, `LIMITE_POR_MINUTO`…) está
documentado en [api/simulaciones.md](api/simulaciones.md).

### Worker (Bun)

| Variable | Obligatoria | Por defecto | Uso |
|---|---|---|---|
| `DATABASE_URL` | sí | | `mysql://usuario:clave@host:3306/bd`. |
| `TOKEN_INTERNO` | sí (16+) | | Debe coincidir con la API. |
| `URL_CALLBACK` | sí | | `http://api:8081` en Compose; `http://127.0.0.1:8081` en local. |
| `HOST_WORKER` | no | `127.0.0.1` | IP del servidor interno; en Compose, `0.0.0.0`. |
| `PUERTO` | no | `7070` | Servidor interno del worker. |
| `HILOS` | no | núcleos − 1 | Hilos de simulación. |
| `TRABAJOS_SIMULTANEOS` | no | `2` | Trabajos en paralelo. |

El resto está en [worker.md](worker.md).

## Docker Compose

`docker-compose.yml` levanta:

| Servicio | Imagen / build | Puerto |
|---|---|---|
| `db` | `mariadb:11.4` | 3306 |
| `migrator` | `mariadb:11.4` + `migraciones/aplicar.sh` | — |
| `api` | `Dockerfile` (raíz, contexto `api/`) | 8080 |
| `worker` | `worker/Dockerfile` | interno 7070 |
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

1. MariaDB 10.6+ con la base y el usuario de `.env`.
2. `cd api && go run ./main.go`
3. `cd worker && bun src/index.ts` (con `worker/.env` o variables exportadas)
4. `cd web && bun run dev`

En local, `URL_CALLBACK` debe ser `http://127.0.0.1:8081`; el puerto `8080` es
el público y no tiene la ruta `/interno`.

## CI

- `.github/workflows/docker-trivy.yml`: tests de Go, worker y web; build de la
  imagen de la API y escaneo con Trivy.
- `Jenkinsfile` (raíz): análisis de seguridad **en paralelo** por componente
  (SonarQube con `api/`, `worker/` y `web/sonar-project.properties`, Hadolint
  para los tres Dockerfiles y Gitleaks global), build multi-arquitectura de las
  tres imágenes, Trivy en paralelo y push a GHCR. Trivy **falla el pipeline**
  si encuentra cualquier vulnerabilidad con corrección disponible en una
  versión posterior (todas las severidades); las que aún no tienen parche se
  ignoran con `--ignore-unfixed`.

## Notas de seguridad

- El servidor interno de la API solo debe escuchar en `0.0.0.0` dentro de una
  red privada de contenedores; nunca publiques `8081`.
- El worker escucha en `127.0.0.1` en local y en `0.0.0.0` dentro de la red de
  Compose; no se publica ningún puerto suyo.
- Cambia `TOKEN_INTERNO`, `SECRETO_COOKIE`, `DB_PASSWORD` y
  `DB_ROOT_PASSWORD` antes de desplegar.
