# PokeTeamCalculatorWorker

Worker de PokeTeamCalculator. Toma de la cola en Redis las peticiones que encola
la API en Go, simula con [pokemon-showdown](https://github.com/smogon/pokemon-showdown)
cada Pokémon del jugador contra el del líder de gimnasio y publica qué
candidatos ganan más. Solo habla con Redis: no conoce MariaDB ni sirve HTTP.

```
Go (API) ──encolar──▶ Redis (sim:cola) ◀──BLMOVE / scripts── worker (Bun)
   ▲                      │
   └──── sim:estados ◀────┘ (procesando, pendiente, completado, error)
```

- `src/sim/`: el simulador, una función pura (validación, IA y combates).
- `src/pool.ts` y `src/sim-hilo.ts`: pool de hilos (`Worker` de Bun). Cada candidato de una petición se simula en un hilo.
- `src/cola.ts`: la cola en Redis (scripts Lua atómicos), latidos y recuperación de los trabajos de workers muertos.
- `src/arranque.ts`: arranque y apagado limpio; `src/index.ts` lo lanza y registra las señales.

El contrato con la API (claves, scripts y estados) está en
[cola-redis.md](cola-redis.md).

## Instalación

Necesitas [Bun](https://bun.sh) 1.3 o superior y Redis 6.2 o superior. El
cliente de Redis viene con Bun: no hay dependencias extra.

```sh
cd worker
bun install
```

## Probar el simulador con el CLI

No necesita Redis.

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

El worker comprueba al arrancar que el servidor es Redis 6.2 o superior (lo
exige `BLMOVE`).

## Variables de entorno

Bun carga `.env` del directorio desde el que se ejecuta. Para desarrollo local,
crea `worker/.env` con `REDIS_URL` o exporta las variables. En Docker Compose,
el servicio `worker` recibe solo estas variables, no todo el `.env` de la raíz.

| Variable | Obligatoria | Por defecto | Para qué |
|---|---|---|---|
| `REDIS_URL` | sí | | `redis://:clave@127.0.0.1:6379/0`. La misma que usa la API |
| `HILOS` | no | núcleos − 1 (mínimo 1) | Hilos de simulación |
| `TRABAJOS_SIMULTANEOS` | no | `2` | Trabajos en curso a la vez |
| `INTERVALO_LATIDO_MS` | no | `5000` | Cada cuánto escribe el worker su latido |
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
`trabajo_completado`, `trabajo_error`, `trabajo_recuperado`, `trabajo_cancelado`,
`trabajo_devuelto`, `apagado`…).

Espera trabajos con un `BLMOVE` bloqueante: los toma en cuanto llegan, sin
sondear. Cada cambio de estado lo publica en el stream `sim:estados`, que la API
aplica en MariaDB. Si Redis se pierde del todo (el cliente agota sus
reconexiones), el worker sale con código 1 y Docker lo reinicia; lo que tuviera
en curso lo recupera otro worker.

Con `SIGTERM` o `SIGINT` deja de tomar trabajos, espera hasta
`ESPERA_APAGADO_MS` a los que están en curso y devuelve **al principio** de la
cola, sin gastar el intento, los que no hayan terminado.

### Estados de un trabajo

- Al tomarlo se gasta un intento y se publica `procesando`.
- `completado`: se publica el `Resultado`.
- `error`: se publica un mensaje para el jugador. Los errores en la petición no
  se reintentan.
- Los demás fallos (hilo caído, timeout…) mandan el trabajo **al final** de la
  cola y publican `pendiente`, hasta agotar `MAX_INTENTOS`.
- `cancelado`: lo marca la API. El worker lo descarta al tomarlo o, si ya lo
  estaba simulando, no publica el resultado.
- Si un worker muere, otro manda sus trabajos al final de la cola cuando caduca
  su latido (lo comprueba al arrancar y cada minuto).

## Probarlo a mano sin la API de Go

```sh
bun src/index.ts &                                                 # con REDIS_URL definida
bun herramientas/encolar.ts herramientas/ejemplo.json              # imprime el id
redis-cli -u "$REDIS_URL" XRANGE sim:estados - +                   # los estados publicados
```

`encolar.ts` mete la petición tal cual, sin validarla, así que también sirve
para probar peticiones erróneas. Usa un id que no choca con los de MariaDB
(o el que indiques con `--id`). Sin la API nadie aplica los estados: se quedan en
el stream.

## Tests

```sh
cd worker
bun test
bunx tsc --noEmit
```

Los tests de la cola necesitan un Redis en `REDIS_URL_TEST`. Cada test usa un
prefijo de claves aleatorio y lo borra al terminar, así que no hace falta una
instancia de usar y tirar. Sin esa variable, se saltan con un aviso. El CI
levanta un Redis para ellos.

```sh
docker run --rm -d -p 127.0.0.1:6379:6379 redis:8.10-alpine
REDIS_URL_TEST=redis://127.0.0.1:6379 bun test
```
