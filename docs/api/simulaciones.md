# Simulaciones

Módulo de la API que recibe una petición de combate, la guarda en MariaDB, la
encola en Redis, informa por SSE de la posición y entrega el resultado que publica
el worker. El contrato de la cola está en [../cola-redis.md](../cola-redis.md).

Los endpoints de catálogo (juegos, líderes, autocompletado y detalle de
Pokémon) están en [catalogo.md](catalogo.md).

## Rutas públicas

| Método | Ruta | Descripción |
|---|---|---|
| `POST` | `/api/simulaciones` | Crea (o reutiliza) un trabajo. |
| `GET` | `/api/simulaciones/{id}` | Estado actual. |
| `DELETE` | `/api/simulaciones/{id}` | Cancela un trabajo pendiente o en proceso. |
| `GET` | `/api/simulaciones/{id}/eventos` | Stream SSE con el estado. |
| `GET` | `/api/salud` | Estado de la base de datos y del worker. |

La identidad del jugador es una cookie anónima `sid` (128 bits, HMAC-SHA256,
`HttpOnly`, `SameSite=Lax`). El valor de `usuario` en la tabla es el SID.

### Crear

```http
POST /api/simulaciones
Content-Type: application/json
```

```json
{
  "gen": "gen4",
  "rival": {
    "especie": "Cranidos",
    "nivel": 14,
    "habilidad": "Mold Breaker",
    "movimientos": ["Headbutt", "Pursuit", "Leer"]
  },
  "candidatos": [
    {
      "especie": "Monferno",
      "nivel": 16,
      "naturaleza": "Adamant",
      "evs": { "atk": 20 },
      "movimientos": ["Mach Punch", "Ember"]
    }
  ]
}
```

- El navegador **no** envía `simulaciones` ni `opciones`: los fija el servidor.
- Cuerpo máximo 64 KB, sin campos desconocidos.
- `gen`: `gen1` … `gen9`; de 1 a 6 candidatos; de 1 a 4 movimientos.
- Cadenas de hasta 60 caracteres, sin caracteres de control.
- Nivel 1–100, EVs 0–252 y 510 en total, IVs 0–31.
- En gen 1 se descartan habilidad, naturaleza y objeto; en gen 2, habilidad y
  naturaleza. En gen 3+ se descarta una habilidad que no exista para esa
  especie/generación (el worker usará la primera).
- `opciones.maxTurnos`, si se envía internamente, debe estar entre 1 y 1000.

Respuestas:

```json
// 202: encolada o duplicada del mismo usuario
{ "id": 123, "estado": "pendiente", "posicion": 3, "estimadoSegundos": 20 }

// 200: cacheada
{ "id": 98, "estado": "completado", "cache": true, "resultado": { } }
```

Si no hay ningún worker con latido reciente (o Redis no responde), la respuesta
`202` añade `"workerDisponible": false`. La petición se guarda igualmente.

Errores: `400 {"error":"campo: mensaje"}`, `404`, `409` (cancelar algo ya
terminado), `429` y `503` (cola llena). El `429` por tener una simulación en
curso incluye su id:

```json
{ "error": "Ya tienes una simulación en cola", "id": 120 }
```

El `429` del límite de peticiones por minuto no lleva `id`.

### Consultar

```json
{ "id": 123, "estado": "pendiente", "posicion": 2, "estimadoSegundos": 14 }
{ "id": 123, "estado": "procesando" }
{ "id": 123, "estado": "completado", "resultado": { } }
{ "id": 123, "estado": "error", "error": "Candidato 1: \"Sylveon\" no existe en gen 4." }
{ "id": 123, "estado": "cancelado" }
```

Los completados son públicos. Los demás estados solo los ve su dueño; para
cualquier otro usuario se responde `404`.

### Cancelar

`DELETE /api/simulaciones/{id}` → `204`. Solo el dueño. Si ya terminó, `409`.
También se puede cancelar un trabajo en `procesando`; el worker descarta el
resultado.

### SSE

`GET /api/simulaciones/{id}/eventos`

```text
event: posicion
data: {"posicion":3,"estimadoSegundos":20}

event: procesando
data: {}

event: completado
data: {"resultado": { }}

event: error
data: {"error": "..."}

event: cancelado
data: {}
```

Al conectar se envía el estado actual. Tras un evento final se cierra la
conexión. Se manda `: ping` cada 15 s. Si un trabajo vuelve a la cola después de
`procesando` (un reintento o un worker muerto), se manda otra vez `posicion`.

Cada instancia de la API tiene su hub. Los cambios que aplica otra le llegan por
Redis (`sim:eventos`) y el sondeo de respaldo cubre los avisos perdidos.

### Salud

`GET /api/salud`

```json
{ "status": "ok", "baseDatos": "ok", "cola": "ok", "workerDisponible": true }
```

Responde `503` solo si falla MariaDB. Sin Redis (`"cola": "error"`) el catálogo
funciona y las simulaciones se guardan; la reconciliación las encola cuando
Redis vuelve.

## Variables de entorno

| Variable | Obligatoria | Por defecto |
|---|---|---|
| `REDIS_URL` | sí | — (`redis://:clave@127.0.0.1:6379/0`) |
| `SECRETO_COOKIE` | sí (mín. 32 bytes) | — |
| `LATIDO_CADUCADO_WORKER` | no | `15s` |
| `SIMULACIONES` | no | `300` |
| `ALEATORIEDAD_RIVAL` | no | `0.2` |
| `TRABAJOS_SIMULTANEOS_WORKER` | no | `2` |
| `MAX_PENDIENTES_POR_USUARIO` | no | `1` |
| `MAX_COLA` | no | `200` |
| `CACHE_TTL` | no | `168h` |
| `RETENCION` | no | `720h` |
| `CANCELAR_SIN_CLIENTE` | no | `2m` (`0` desactiva) |
| `INTERVALO_SONDEO_SSE` | no | `5s` |
| `LIMITE_POR_MINUTO` | no | `10` |
| `PROXIES_CONFIABLES` | no | vacío (IPs/CIDR separados por comas) |

La conexión a MariaDB se reutiliza de `database.Initialize()` (`DB_HOST`,
`DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_TLS`); no hace falta
`DATABASE_DSN`.

## Contrato con el worker

Todo pasa por Redis; las claves, los scripts y los estados están en
[../cola-redis.md](../cola-redis.md). Desde el lado de la API:

- Tabla: la crea `migraciones/001_simulaciones.sql` (raíz del repositorio). Solo
  la usa la API: el worker no se conecta a MariaDB.
- Migraciones propias:
  - `migraciones/002_indices_simulaciones.sql`: índices `idx_usuario_estado` e
    `idx_hash_estado`.
  - `migraciones/003_v_gym_teams_formas.sql`: corrige las formas de
    `v_gym_teams`. Requiere que pokedata haya creado sus vistas.
- Al crear, la API inserta la fila y ejecuta `encolar`: el trabajo queda al final
  de `sim:cola`. Si Redis falla, la creación sigue y la reconciliación lo encola
  después.
- La API aplica en MariaDB los estados que el worker publica en `sim:estados`
  (grupo `api`) y los confirma con `XACK` + `XDEL`. Ninguno pisa un estado final.
- La posición sale de `sim:cola`. Si Redis no la conoce (no responde, o un worker
  acaba de tomar el trabajo y aún no ha llegado su `procesando`), se da como `1`.
- Cancelar marca la fila en MariaDB y ejecuta `cancelar` en Redis: el trabajo
  sale de la cola o, si ya estaba en curso, el worker no publica su resultado.
- `workerDisponible`: hay algún latido en `sim:workers` más reciente que
  `LATIDO_CADUCADO_WORKER`. Se comprueba cada 15 s y, si no se veía ningún
  worker, también al crear.

### Formas regionales en `v_gym_teams`

`pokemon_en` usaba el nombre de la especie, así que se perdía la forma. La
migración `003` reconstruye el nombre de Showdown desde `p.identifier`. Con los
datos actuales de pokedata había **5 filas / 5 formas** afectadas:

| `pokemon` | Antes | Ahora |
|---|---|---|
| `yamask-galar` | Yamask | Yamask-Galar |
| `weezing-galar` | Weezing | Weezing-Galar |
| `darmanitan-galar-standard` | Darmanitan | Darmanitan-Galar |
| `toxtricity-low-key` | Toxtricity | Toxtricity-Low-Key |
| `meowstic-female` | Meowstic | Meowstic-Female |

La consulta de diagnóstico está al final de la migración. Si pokedata vuelve a
importar, recrea sus vistas: hay que reaplicar `003` después.

## Ejemplo desde el navegador

```js
const r = await fetch('/api/simulaciones', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(peticion),
});
const datos = await r.json();
if (datos.estado === 'completado') mostrar(datos.resultado);
else {
  const es = new EventSource(`/api/simulaciones/${datos.id}/eventos`);
  es.addEventListener('posicion', e => mostrarPosicion(JSON.parse(e.data)));
  es.addEventListener('procesando', () => mostrarProcesando());
  es.addEventListener('completado', e => {
    mostrar(JSON.parse(e.data).resultado);
    es.close();
  });
  es.addEventListener('error', e => {
    if (e.data) mostrarError(JSON.parse(e.data).error);
    es.close();
  });
  es.addEventListener('cancelado', () => es.close());
}
```

`EventSource` también dispara `error` al cortarse la conexión, sin `data`; en
ese caso reconecta solo.

## Pruebas

```bash
cd api
go test ./...
```

Las de Redis usan [miniredis](https://github.com/alicebob/miniredis): no hace
falta ningún servidor. Las pruebas con MariaDB se omiten si no existe
`DATABASE_DSN_TEST`. Para ejecutarlas:

```bash
cd api
DATABASE_DSN_TEST='usuario:clave@tcp(127.0.0.1:3306)/poketeam_test?parseTime=true' go test ./...
```

## Decisiones

- **Concurrencia en la creación:** se usa `GET_LOCK` de MariaDB
  (`poketeam_simulaciones_crear`) sobre una conexión dedicada mientras se
  comprueban caché, duplicado y límites y se inserta. Es un bloqueo global muy
  corto: serializa las creaciones y evita que dos peticiones simultáneas se
  salten los límites, incluso con varias instancias de la API.
- **Cola:** la creación no depende de Redis ni de que haya un worker vivo: la
  fila se guarda en MariaDB y, si `encolar` falla, la reconciliación la encola.
- **Reconciliación:** al arrancar y cada 30 s, con `sim:estados` vacío, la API
  vuelve a encolar (al final, por orden de creación) los trabajos activos que
  Redis no conoce y saca de la cola los que ya terminaron.
- **SSE:** servidor HTTP sin `WriteTimeout`. Los cambios llegan al aplicar los
  estados del worker y, en las demás réplicas, por Pub/Sub; el sondeo de
  respaldo cada `INTERVALO_SONDEO_SSE` cubre los avisos perdidos.
- **Trabajos abandonados:** se cancelan los `pendiente` sin suscriptores SSE y
  con más de `CANCELAR_SIN_CLIENTE` desde `creado`; también salen de la cola de
  Redis. Con varias réplicas, cada una solo ve sus propios suscriptores.
- **Limpieza:** cada hora se borran `error`/`cancelado` de más de 7 días y
  `completado` de más de `RETENCION`. Nunca se tocan `pendiente` ni
  `procesando`.
