# Cola en Redis

Contrato entre la API (Go) y el worker (Bun). Si cambias una clave, un campo o un
script, cambia a la vez este documento, `api/endpoints/simulaciones/cola.go` y
`worker/src/cola.ts`.

## Reparto

- **MariaDB** es la fuente de verdad y solo la usa la API: caché, duplicados,
  límites, estado, resultado e historial (tabla `simulaciones`).
- **Redis** guarda el orden de la cola, lo que cada worker tiene en curso, los
  latidos de los workers y los cambios de estado que el worker manda a la API.
- La API y el worker no se hablan por HTTP. El worker no conoce MariaDB.

```text
web ──/api──▶ api ──SQL──▶ MariaDB
               │ ▲
     encolar,  │ │  sim:estados
     cancelar  ▼ │
              Redis ◀──BLMOVE / scripts── worker
```

## Requisitos

- Redis 6.2 o superior (`BLMOVE`, `XAUTOCLAIM`). Docker Compose usa
  `redis:8.10-alpine`. Valkey también sirve: es el mismo protocolo.
- Sin Cluster: el script `recuperar` calcula claves dentro del propio script.
- Persistencia AOF (Compose la activa). Si aun así Redis pierde los datos, la API
  reconstruye la cola desde MariaDB (ver [Reconciliación](#reconciliación)).

## Claves

| Clave | Tipo | Contenido |
|---|---|---|
| `sim:cola` | lista | Ids pendientes. El worker toma por la derecha (`BLMOVE … RIGHT LEFT`): `LPUSH` deja un trabajo **al final** y `RPUSH`, **al principio**. Posición = longitud − índice. |
| `sim:procesando:{worker}` | lista | Ids que ese worker tiene en curso. |
| `sim:trabajo:{id}` | hash | `peticion` (JSON), `intentos` y, como lápida, `cancelado` o `fin`. TTL de 24 h al encolarlo y de 1 h al terminar o cancelarse. |
| `sim:workers` | zset | Id del worker → último latido en ms, con el `TIME` de Redis. |
| `sim:estados` | stream | Cambios de estado del worker para la API (grupo de consumo `api`). |
| `sim:eventos` | Pub/Sub | `{"id":…,"origen":…}`: avisa a las demás réplicas de la API de que un trabajo cambió. |

El id de un worker es `host-pid-aleatorio` y el de una réplica de la API, igual.

## Dónde vuelve cada trabajo

| Situación | Quién lo mueve | Posición | Intento |
|---|---|---|---|
| Fallo reintentable (hilo caído, timeout…) | el worker (`fallar`) | **al final** | se gasta |
| El worker muere | otro worker, cuando caduca el latido (`recuperar`) | **al final** | se gasta |
| Apagado limpio (`SIGTERM`) | el worker (`devolver`) | al principio | no se gasta |
| Petición inválida o `MAX_INTENTOS` agotados | el worker | sale de la cola: `error` | — |
| Cancelado por el jugador o abandonado | la API (`cancelar`) | sale de la cola | — |

## Scripts

Cada uno es un script Lua y se ejecuta de forma atómica.

| Script | Lo ejecuta | Qué hace |
|---|---|---|
| `encolar` | API (crear y reconciliar) | Si `trabajo:{id}` no existe, crea el hash y hace `LPUSH cola`; devuelve la posición. Si existe, aunque sea una lápida, no hace nada. |
| `cancelar` | API | `LREM cola` y marca `cancelado` con TTL de 1 h. Si ya estaba en curso, el worker no publicará nada. |
| `empezar` | worker, justo después del `BLMOVE` | Si el hash falta, está cancelado o terminado, lo saca de su lista y lo descarta. Si no, `HINCRBY intentos`, publica `procesando` y devuelve la petición. |
| `completar` | worker | Solo si el id sigue en su lista: lo saca, marca `fin` y publica `completado` con el resultado (salvo que esté cancelado). |
| `fallar` | worker | Igual, y si es reintentable y quedan intentos: `LPUSH cola` y publica `pendiente` (`motivo=reintento`). Si no, publica `error`. |
| `devolver` | worker al apagarse | `RPUSH cola`, resta el intento y publica `pendiente` (`motivo=devuelto`). |
| `latir` | worker, cada `INTERVALO_LATIDO_MS` | `ZADD workers <TIME> worker`. |
| `recuperar` | worker, al arrancar y cada 60 s | Para cada worker (menos él) con el latido más viejo que `LATIDO_CADUCADO_MS`: vacía su lista al final de la cola (`motivo=caducado`) o publica `error` si agotó `MAX_INTENTOS`; los cancelados se descartan. Después lo borra de `workers`. |
| `baja` | worker al apagarse | Se borra de `workers` solo si su lista está vacía; si no, otro worker la recupera cuando caduque el latido. |

Además, cada worker revisa en cada recuperación su propia lista: un id que siga
ahí sin estar simulándose en dos pasadas seguidas (falló Redis al publicar el
final) se da por fallido y vuelve al final de la cola.

## Estados (`sim:estados`)

| Campo | Cuándo | Valor |
|---|---|---|
| `id` | siempre | Id de la fila en MariaDB. |
| `tipo` | siempre | `procesando`, `pendiente`, `completado` o `error`. |
| `intentos` | siempre | Intentos gastados. |
| `worker` | siempre | Id del worker. |
| `motivo` | `pendiente` | `reintento`, `devuelto` o `caducado`. |
| `resultado` | `completado` | JSON del `Resultado` (`worker/src/sim/tipos.ts`). |
| `error` | `error` | Mensaje para el jugador. |

La API lee el stream con el grupo `api` (`XREADGROUP`, bloqueando 5 s) y aplica
cada estado en MariaDB:

- `procesando` fija `estado`, `iniciado`, `worker` e `intentos`, salvo que el
  trabajo ya haya terminado.
- `pendiente` lo devuelve a `pendiente` si estaba `procesando`.
- `completado` y `error` lo cierran solo si seguía `pendiente` o `procesando`: un
  cancelado nunca se pisa.
- Un estado inválido (id o `tipo` desconocidos) se registra y se confirma para no
  atascar el stream. Un `resultado` que no es JSON (o pasa de 4 MB) cierra el
  trabajo en `error` con el mensaje de error interno.
- Si falla MariaDB, el estado no se confirma: cada 30 s la API reclama
  (`XAUTOCLAIM`) los que llevan más de 1 minuto sin confirmar.
- Al aplicarlo hace `XACK` y `XDEL`, así que el stream solo guarda lo pendiente.

Si Redis pierde el grupo, la API lo vuelve a crear desde el principio del
stream. Con varias réplicas, un estado intermedio puede verse desordenado un
momento; los finales nunca se pisan.

## Avisos entre réplicas (`sim:eventos`)

La réplica que aplica un estado avisa a su hub SSE y publica `{"id":…,"origen":…}`.
Las demás leen el trabajo de MariaDB y avisan a sus suscriptores; ignoran sus
propios avisos. Pub/Sub no guarda nada: el sondeo de respaldo del SSE
(`INTERVALO_SONDEO_SSE`) cubre los avisos perdidos.

## Workers vivos

- Cada worker escribe su latido en `sim:workers` cada `INTERVALO_LATIDO_MS`, con
  la hora de Redis para que no importe el reloj de cada máquina.
- La API da por disponible un worker con un latido más reciente que
  `LATIDO_CADUCADO_WORKER` (15 s). Lo comprueba cada 15 s y, si no veía ninguno,
  también al crear una simulación.
- Los workers recuperan los trabajos de los que llevan más de
  `LATIDO_CADUCADO_MS` (60 s) sin latir.

## Reconciliación

La API, al arrancar y cada 30 s, y solo si `sim:estados` está vacío (para no
adelantarse a un final que aún no ha aplicado):

1. Vuelve a encolar, al final y en orden de creación, las filas `pendiente` o
   `procesando` de más de 10 s que no tienen `sim:trabajo:{id}`. Las que estaban
   en `procesando` pasan a `pendiente`, porque ningún worker podrá publicarlas.
2. Cancela en Redis los ids de `sim:cola` cuya fila ya terminó o no existe.

Cubre Redis caído al crear una simulación, la pérdida de datos de Redis y la
migración desde la cola antigua en MariaDB. Las lápidas evitan volver a encolar
algo que acaba de terminar o cancelarse.

## Inspeccionar

```sh
docker compose exec redis sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli LRANGE sim:cola 0 -1'   # izquierda = final
docker compose exec redis sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ZRANGE sim:workers 0 -1 WITHSCORES'
docker compose exec redis sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli XLEN sim:estados'
docker compose exec redis sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli HGETALL sim:trabajo:123'
```

## Pruebas

- Worker: `test/cola.test.ts` necesita un Redis en `REDIS_URL_TEST`. Usa un
  prefijo de claves aleatorio por test y lo borra, así que no pisa otros datos.
  El CI levanta un Redis para estos tests.
- API: usa [miniredis](https://github.com/alicebob/miniredis) y no necesita Redis.
