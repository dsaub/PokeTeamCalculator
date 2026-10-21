# PokeTeamCalculatorWorker

Worker de PokeTeamCalculator. Toma de la cola de MariaDB las peticiones que
guarda la API en Go, simula con [pokemon-showdown](https://github.com/smogon/pokemon-showdown)
cada Pokémon del jugador contra el del líder de gimnasio y guarda qué
candidatos ganan más.

```
Go (API) ──INSERT──▶ MariaDB (simulaciones) ◀──toma/actualiza── worker (Bun)
   ▲   └──POST /despertar───────────────────────────────────────────▶│
   └────────────POST /interno/simulaciones/{id}/lista ◀───────────────┘
```

- `src/sim/`: el simulador, una función pura (validación, IA y combates).
- `src/pool.ts` y `src/sim-hilo.ts`: pool de hilos (`Worker` de Bun). Cada candidato de una petición se simula en un hilo.
- `src/cola.ts`: la cola en MariaDB (`FOR UPDATE SKIP LOCKED`), latidos, recuperación y aviso a Go.
- `src/index.ts`: arranque, servidor HTTP interno y apagado limpio.

## Instalación

Necesitas [Bun](https://bun.sh) 1.3 o superior y MariaDB 10.6 o superior.

```sh
cd worker
bun install
```

## Probar el simulador con el CLI

No necesita base de datos.

```sh
bun herramientas/cli.ts herramientas/ejemplo.json            # tabla de resultados
bun herramientas/cli.ts herramientas/ejemplo.json --json     # el Resultado tal como lo verá Go
bun herramientas/cli.ts herramientas/ejemplo.json --textos   # los textos de exportación de Showdown
cat peticion.json | bun herramientas/cli.ts                  # lee la petición de stdin
```

Si la petición no es válida, muestra el motivo y sale con código 2.

El formato de la petición y del resultado está en `src/sim/tipos.ts`, que es el
contrato con Go. Los nombres de especies, movimientos, habilidades, objetos y
naturalezas van en inglés, como en Showdown (`"Mr. Mime"`, `"Mach Punch"`). Las
formas regionales llevan su sufijo (`"Yamask-Galar"`).

## Crear la tabla

Desde la raíz del repositorio:

```sh
mariadb -u usuario -p poketeam < migraciones/001_simulaciones.sql
```

Si estás en `worker/`, usa `../migraciones/001_simulaciones.sql`.

El worker comprueba al arrancar que el servidor es MariaDB 10.6 o superior (lo
exige `SKIP LOCKED`).

Los datos de Pokémon y de los líderes de gimnasio que usa la API los genera
[pokedata](https://github.com/dsaub/pokedata) (`pokedata-gen --mode mysql …`).
El worker no lee esas tablas.

## Variables de entorno

Bun carga `.env` del directorio desde el que se ejecuta. Copia el `.env.example`
de la raíz a `worker/.env` para desarrollo local, o exporta las variables (Docker
Compose usa el `.env` de la raíz).

| Variable | Obligatoria | Por defecto | Para qué |
|---|---|---|---|
| `DATABASE_URL` | sí | | `mysql://usuario:clave@host:3306/bd` |
| `TOKEN_INTERNO` | sí | | Secreto compartido con Go (16 caracteres o más) |
| `URL_CALLBACK` | sí | | URL base del servidor **interno** de la API: `http://127.0.0.1:8081` en local, `http://api:8081` en Compose. Nunca `8080` |
| `HOST_WORKER` | no | `127.0.0.1` | IP del servidor interno. En Compose, `0.0.0.0` para que la API lo alcance. |
| `PUERTO` | no | `7070` | Puerto del servidor interno. |
| `HILOS` | no | núcleos − 1 (mínimo 1) | Hilos de simulación |
| `TRABAJOS_SIMULTANEOS` | no | `2` | Trabajos en curso a la vez |
| `INTERVALO_SONDEO_MS` | no | `1000` | Cada cuánto se mira la cola si está vacía |
| `INTERVALO_LATIDO_MS` | no | `5000` | Cada cuánto se actualiza el latido de los trabajos en curso |
| `LATIDO_CADUCADO_MS` | no | `60000` | Latido más antiguo que esto: el worker se da por muerto |
| `MAX_INTENTOS` | no | `3` | Intentos por trabajo antes de marcarlo como error |
| `TIMEOUT_TAREA_MS` | no | `60000` | Tiempo máximo de un candidato en un hilo |
| `ESPERA_APAGADO_MS` | no | `30000` | Cuánto se espera a los trabajos en curso al apagar |

Si falta alguna obligatoria o hay valores inválidos, el worker no arranca y dice
cuáles son.

## Arrancar el worker

```sh
cd worker
bun src/index.ts        # o bun start
```

Escribe una línea JSON por evento en stdout (`arranque`, `trabajo_tomado`,
`trabajo_completado`, `trabajo_error`, `trabajo_recuperado`, `callback_fallido`,
`apagado`…).

Servidor interno (por defecto en `127.0.0.1:${PUERTO}`; en Compose,
`0.0.0.0:${PUERTO}` dentro de la red privada):

- `POST /despertar` con `Authorization: Bearer ${TOKEN_INTERNO}`: busca trabajos al instante. Go lo llama al encolar.
- `GET /salud`: hilos totales y ocupados, trabajos en curso y si MariaDB responde. Devuelve 503 si MariaDB no contesta en 2 s.

Cuando un trabajo termina (completado o error), el worker hace
`POST ${URL_CALLBACK}/interno/simulaciones/{id}/lista` con el mismo token y sin
cuerpo. Si falla tras 3 reintentos, lo registra y sigue (la API también sondea
la tabla). El callback debe apuntar al puerto interno (`8081`), no al público.

Con `SIGTERM` o `SIGINT` deja de tomar trabajos, espera hasta
`ESPERA_APAGADO_MS` a los que están en curso y devuelve a `pendiente`, sin
gastar el intento, los que no hayan terminado.

### Estados de un trabajo

- `pendiente` → `procesando` al tomarlo (`intentos + 1`).
- `completado`: `resultado` tiene el `Resultado`.
- `error`: `error` tiene un mensaje para el jugador. Los errores en la petición no se reintentan. Los demás vuelven a `pendiente` hasta agotar `MAX_INTENTOS`.
- `cancelado`: lo pone Go. El worker se salta el trabajo y nunca lo sobrescribe.
- Si un worker muere, otro devuelve sus trabajos a `pendiente` cuando caduca el latido (lo comprueba cada minuto).

## Probarlo a mano sin la API de Go

```sh
bun src/index.ts &                                                 # con .env configurado
bun herramientas/encolar.ts herramientas/ejemplo.json --despertar  # imprime el id
mariadb -e "SELECT id, estado, error, JSON_PRETTY(resultado) FROM simulaciones ORDER BY id DESC LIMIT 1" poketeam
```

`encolar.ts` inserta el JSON tal cual, sin validarlo, así que también sirve para
probar peticiones erróneas. Usa `DATABASE_URL` y, con `--despertar`, `PUERTO` y
`TOKEN_INTERNO`.

## Tests

```sh
cd worker
bun test
bunx tsc --noEmit
```

Los tests de la cola necesitan una MariaDB de usar y tirar en
`DATABASE_URL_TEST`, porque borran y recrean la tabla `simulaciones`. Sin esa
variable, se saltan con un aviso.

```sh
DATABASE_URL_TEST=mysql://usuario:clave@127.0.0.1:3306/poketeam_test bun test
```
