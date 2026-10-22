# Arquitectura

## Componentes

| Componente | Tecnología | Puerto | Responsabilidad |
|---|---|---|---|
| `web/` | React 19 + Vite 8 + TypeScript | 5173 (dev) / 80 (Docker) | Interfaz: elegir juego y líder, construir equipo, seguir la simulación y mostrar la recomendación. |
| `api/` | Go 1.27 + Gin + sqlc | 8080 | Catálogo de pokedata, creación/consulta/cancelación de simulaciones, SSE y salud. Única que toca MariaDB. |
| `worker/` | Bun + Pokémon Showdown | — | Toma trabajos de Redis, simula cada candidato en hilos y publica el resultado en Redis. |
| MariaDB | 10.6+ | 3306 | Tabla `simulaciones` (fuente de verdad) y datos de pokedata. |
| Redis | 6.2+ (Compose: 8.10) | 6379 | Cola, trabajos en curso, latidos de los workers y estados que el worker manda a la API. |

## Flujo de una simulación

1. La web construye la petición (gen, rival y 1–6 candidatos) y hace
   `POST /api/simulaciones`.
2. La API sanea la petición, calcula su hash, busca caché/duplicado, comprueba
   los límites e inserta la fila en `simulaciones`.
3. La API pone el trabajo al final de la cola de Redis y responde `202` con su
   posición.
4. El worker lo toma con `BLMOVE` (sin sondeo), publica `procesando` en el
   stream `sim:estados`, simula cada candidato en paralelo y publica el
   `Resultado`.
5. La API lee `sim:estados`, guarda cada cambio en MariaDB y lo publica por SSE.
   Las demás réplicas se enteran por Pub/Sub; un sondeo de respaldo cada
   `INTERVALO_SONDEO_SSE` cubre los avisos perdidos.
6. La web muestra el ranking y explica la recomendación.

Si el worker falla con un error reintentable o muere, el trabajo vuelve **al
final** de la cola y se reintenta cuando terminan los que ya esperaban. El
detalle está en [cola-redis.md](cola-redis.md).

## Contratos

- **API ↔ worker** (solo a través de Redis, ver [cola-redis.md](cola-redis.md)):
  - Claves, scripts y estados en `api/endpoints/simulaciones/cola.go` y
    `worker/src/cola.ts`.
  - `Peticion` y `Resultado` están definidos en
    `api/endpoints/simulaciones/peticion.go` y `worker/src/sim/tipos.ts`.
  - Sin token ni HTTP entre ellos; Redis lleva su contraseña en `REDIS_URL`.
- **Web ↔ API**:
  - Tipos en `web/src/api/tipos.ts`.
  - Mismo origen bajo `/api`; en desarrollo, proxy de Vite.
- **Base de datos**:
  - Migraciones en `migraciones/` (ver [migraciones.md](migraciones.md)).
  - Solo la API se conecta, con `database.Initialize()`.

## Decisiones relevantes

- **MariaDB manda, Redis reparte**: si Redis pierde datos o no responde al
  crear, la reconciliación de la API vuelve a encolar desde MariaDB.
- **Reintentos al final**: un fallo reintentable o un worker muerto mandan el
  trabajo al final de la cola; un apagado limpio lo devuelve al principio sin
  gastar intento.
- **SSE + Pub/Sub + sondeo**: cada réplica tiene su hub; los cambios llegan a
  todas por Redis y el sondeo cubre los avisos perdidos.
- **Identidad anónima**: cookie `sid` firmada con HMAC; no hay cuentas.
- **Caché por hash**: misma petición normalizada devuelve el resultado
  completado dentro de `CACHE_TTL`.
- **Un worker puede morir**: cuando caduca su latido, otro worker devuelve sus
  trabajos al final de la cola hasta agotar `MAX_INTENTOS`.
- **Formas regionales**: la vista `v_gym_teams` se corrige con la migración
  `003` para conservar el sufijo de Showdown (`Yamask-Galar`).

## Deuda conocida

- El catálogo no expone objetos ni naturalezas; el formulario web no los ofrece.
- `v_gym_teams` la recrea pokedata en cada importación: hay que reaplicar la
  migración `003` después.
- Con varias réplicas de la API, cada una solo ve sus propios suscriptores SSE
  al buscar trabajos abandonados (`CANCELAR_SIN_CLIENTE`): podría cancelar uno
  que tiene el SSE abierto en otra réplica.
