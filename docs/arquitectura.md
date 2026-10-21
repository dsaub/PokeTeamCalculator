# Arquitectura

## Componentes

| Componente | Tecnología | Puerto | Responsabilidad |
|---|---|---|---|
| `web/` | React 19 + Vite 8 + TypeScript | 5173 (dev) / 80 (Docker) | Interfaz: elegir juego y líder, construir equipo, seguir la simulación y mostrar la recomendación. |
| `api/` | Go 1.27 + Gin + sqlc | 8080 público, 8081 interno | Catálogo de pokedata, creación/consulta/cancelación de simulaciones, SSE y salud. |
| `worker/` | Bun + Pokémon Showdown | 7070 (solo interno) | Toma trabajos de MariaDB, simula cada candidato en hilos y escribe el resultado. |
| MariaDB | 10.6+ | 3306 | Cola `simulaciones` y datos de pokedata. |

## Flujo de una simulación

1. La web construye la petición (gen, rival y 1–6 candidatos) y hace
   `POST /api/simulaciones`.
2. La API sanea la petición, calcula su hash, busca caché/duplicado, comprueba
   los límites e inserta la fila en `simulaciones`.
3. La API responde `202` y, en segundo plano, hace `POST /despertar` al worker.
4. El worker toma el trabajo con `FOR UPDATE SKIP LOCKED`, simula cada candidato
   en paralelo y guarda el `Resultado`.
5. El worker avisa a la API con `POST /interno/simulaciones/{id}/lista`.
6. La API publica el evento final por SSE; si el callback se pierde, un sondeo
   de respaldo cada `INTERVALO_SONDEO_SSE` lo detecta.
7. La web muestra el ranking y explica la recomendación.

## Contratos

- **API ↔ worker**:
  - La API escribe la fila; el worker solo la lee/actualiza.
  - `Peticion` y `Resultado` están definidos en
    `api/endpoints/simulaciones/peticion.go` y `worker/src/sim/tipos.ts`.
  - Token compartido `TOKEN_INTERNO` en `Authorization: Bearer`.
- **Web ↔ API**:
  - Tipos en `web/src/api/tipos.ts`.
  - Mismo origen bajo `/api`; en desarrollo, proxy de Vite.
- **Base de datos**:
  - Migraciones en `migraciones/` (ver [migraciones.md](migraciones.md)).
  - La API reutiliza la conexión de `database.Initialize()`; el worker usa
    `DATABASE_URL`.

## Decisiones relevantes

- **SSE + sondeo**: el hub SSE vive en un proceso; el sondeo cubre callbacks
  perdidos y el paso a `procesando`.
- **Identidad anónima**: cookie `sid` firmada con HMAC; no hay cuentas.
- **Caché por hash**: misma petición normalizada devuelve el resultado
  completado dentro de `CACHE_TTL`.
- **Un worker puede morir**: los latidos caducados devuelven el trabajo a
  `pendiente` hasta agotar `MAX_INTENTOS`.
- **Formas regionales**: la vista `v_gym_teams` se corrige con la migración
  `003` para conservar el sufijo de Showdown (`Yamask-Galar`).

## Deuda conocida

- El catálogo no expone objetos ni naturalezas; el formulario web no los ofrece.
- `v_gym_teams` la recrea pokedata en cada importación: hay que reaplicar la
  migración `003` después.
- El hub SSE no se comparte entre réplicas de la API; el sondeo mitiga el
  problema, pero no lo elimina.
