# Lo que le falta a la API de Pokémon

> **Documento histórico.** Describe la API antigua `/api/v1/pokemon`. El producto
> actual usa el catálogo `/api/*` documentado en
> [../api/catalogo.md](../api/catalogo.md). Varias carencias de esta lista ya se
> corrigieron o dejaron de ser relevantes; se marcan abajo.

Estado observado el 20/10/2026 contra `http://localhost:8080/api/v1/pokemon` (primera revisión: 19/10/2026).

Cada tarea es independiente: se puede hacer, probar con el `curl` de **Cómo comprobarlo** y marcar, sin tocar las demás. Están ordenadas de más fácil y útil a más trabajosa.

## Cómo está ahora

| Petición | Respuesta actual |
|---|---|
| `GET /api/v1/pokemon?page=N` | `{"page", "pokemons", "last", "elements"}`, 20 por página, 68 páginas (1351 Pokémon con formas) |
| `GET /api/v1/pokemon?page=abc` | `400` en JSON ✅ |
| `GET /api/v1/pokemon?page=0`, `?page=-1`, `?page=999` | **200 con `pokemons: null`** (y `last: false` para 0 y -1) |
| `GET /api/v1/pokemon/:id` | el Pokémon; `404` en JSON si no existe ✅; `400` si no es numérico ✅ |
| `GET /api/status` | **404** (ahora en JSON) |
| Rutas desconocidas | `404` en JSON ✅ |
| Parámetros `search`, `name`, `type`, `default`, `page_size`, `limit`, `size`… | ignorados |
| CORS | sin cabeceras `Access-Control-*`; `OPTIONS` responde `404` |

Forma de los errores (nueva):

```json
{"error": "pokemon_not_found", "message": "the specified pokemon was not found", "status": 404}
```

Ejemplo de un Pokémon tal como llega hoy:

```json
{"id": 658, "identifier": "greninja", "species_id": 658, "height": 15, "weight": 400,
 "base_experience": 239, "sort_order": 788, "is_default": 1,
 "type_ids": "11, 17", "type_names": "Agua, Siniestro"}
```

---

## Tareas

- [x] 1. Endpoint de estado — existe `GET /api/salud` (base de datos y worker)
- [x] 2. 404 cuando el Pokémon no existe; el 400 ahora usa `bad_pokemon_id`
- [x] 3. Página vacía como `[]` y validación de `page` (`page < 1` → 400)
- [x] 4. Total de resultados en el listado (`total`, `page_size`, `total_pages`)
- [~] 5. Tipos como listas, no como texto — el catálogo nuevo (`/api/pokemon`) ya los devuelve como array; `/api/v1` sigue con texto
- [x] 6. Errores en JSON
- [ ] 7. Tamaño de página configurable
- [ ] 8. Búsqueda por nombre
- [ ] 9. Filtros por tipo y por formas
- [ ] 10. Nombre en español para mostrar
- [x] 11. Preparado para producción — nginx sirve web y API bajo el mismo origen (ver `web/nginx.conf`)
- [ ] 12. Imagen de cada Pokémon

### 1. Endpoint de estado

**Problema:** la página `/estado` del frontend consulta `GET /api/status` para saber si el backend está listo, y sigue devolviendo 404 (ahora en JSON, `endpoint_not_found`), así que siempre muestra error. Tampoco existen `/api/v1/status` ni `/api/health`.

**Propuesta:** `GET /api/status` → `200` con

```json
{"status": "ok"}
```

Valores: `"starting"` mientras arranca (p. ej. cargando la base de datos), `"ok"` cuando ya responde y `"error"` si algo falla (en ese caso mejor con código `503`).

**Cómo comprobarlo:**

```sh
curl -i http://localhost:8080/api/status   # 200 y {"status":"ok"}
```

### 2. 404 cuando el Pokémon no existe ✅

**Hecho (20/10/2026):** `99999`, `0` y `-1` → `404 pokemon_not_found`; `pikachu` → `400`; `25` → `200`.

**Hecho:** el `400` de `/api/v1/pokemon/pikachu` ahora responde `bad_pokemon_id`. Antes reutilizaba el error de la paginación:

```json
{"error": "bad_page_entry", "message": "page parameter needs number", "status": 400}
```

Debería hablar del id, p. ej. `{"error": "bad_pokemon_id", "message": "pokemon id needs number", "status": 400}`.

**Problema original:** `GET /api/v1/pokemon/99999` y `GET /api/v1/pokemon/pikachu` devuelven `200` con el cuerpo vacío. El cliente no distingue entre «no existe» y «error raro».

**Propuesta:**
- Id numérico que no existe → `404`.
- Id no numérico → `400`.

**Cómo comprobarlo:**

```sh
curl -i http://localhost:8080/api/v1/pokemon/99999    # 404
curl -i http://localhost:8080/api/v1/pokemon/pikachu  # 400
curl -i http://localhost:8080/api/v1/pokemon/25       # 200, sigue igual
```

### 3. Página vacía como `[]` y validación de `page`

**Hecho (21/10/2026):**
- `?page=abc`, `?page=0` y `?page=-1` → `400 bad_page_entry`.
- Fuera de rango (`?page=69`, `?page=999`) → `pokemons: []` y `last: true`.
- Sin `page` → página 1.

**Cómo comprobarlo:**

```sh
curl -s "http://localhost:8080/api/v1/pokemon?page=999"  # "pokemons":[] y "last":true
curl -i "http://localhost:8080/api/v1/pokemon?page=0"    # 400
curl -i "http://localhost:8080/api/v1/pokemon?page=-1"   # 400
curl -i "http://localhost:8080/api/v1/pokemon?page=abc"  # 400 (ya funciona)
```

### 4. Total de resultados en el listado

**Hecho (21/10/2026):** el listado incluye:

```json
{"page": 68, "page_size": 20, "total": 1351, "total_pages": 68, "last": true, "pokemons": [...]}
```

Se eliminó `elements`, que siempre valía 20.

**Cómo comprobarlo:**

```sh
curl -s "http://localhost:8080/api/v1/pokemon?page=1" | grep -o '"total":[0-9]*'   # "total":1351
```

### 5. Tipos como listas, no como texto

**Problema:** los tipos llegan como texto separado por comas (`"type_names": "Agua, Siniestro"`, `"type_ids": "11, 17"`) y el cliente tiene que partirlos.

**Propuesta:**

```json
{"types": [{"id": 11, "name": "Agua"}, {"id": 17, "name": "Siniestro"}]}
```

Mantener el orden del juego (primer tipo primero). Los nombres actuales ya coinciden con los que usa el frontend para calcular ventajas, así que no hay que cambiarlos.

**Cómo comprobarlo:**

```sh
curl -s http://localhost:8080/api/v1/pokemon/658   # "types":[{"id":11,"name":"Agua"},{"id":17,"name":"Siniestro"}]
```

### 6. Errores en JSON ✅

**Hecho (20/10/2026):** todos los errores observados (`400`, `404` de Pokémon y de ruta desconocida) llegan con `Content-Type: application/json; charset=utf-8` y esta forma, que pasa a ser la de referencia en lugar de la propuesta original:

```json
{"error": "endpoint_not_found", "message": "The requested resource does not exist", "status": 404}
```

Detalles menores, no bloquean:
- Los mensajes mezclan mayúscula inicial (`The requested…`) y minúscula (`the specified…`).
- Un método no soportado (`POST /api/v1/pokemon/25`) responde `404` en vez de `405`.

**Problema original:** las rutas desconocidas responden `404 page not found` en `text/plain`. Mezclar formatos obliga al cliente a comprobar el tipo de contenido antes de leer la respuesta.

**Propuesta:** todos los errores con `Content-Type: application/json` y la misma forma:

```json
{"error": {"code": "not_found", "message": "No existe el Pokémon 99999"}}
```

**Cómo comprobarlo:**

```sh
curl -i http://localhost:8080/api/v1/noexiste   # 404, Content-Type: application/json
```

### 7. Tamaño de página configurable

**Problema:** con 20 por página, descargar la Pokédex entera son 68 peticiones, y el navegador solo abre unas 6 a la vez contra el mismo servidor. Hoy tarda unos 9 s.

**Propuesta:** `?page_size=N`, por defecto 20 y con un máximo (p. ej. 200). Si se pide más del máximo, responder con el máximo o con `400`, pero no ignorarlo en silencio.

**Cómo comprobarlo:**

```sh
curl -s "http://localhost:8080/api/v1/pokemon?page=1&page_size=100" | grep -o '"id":' | wc -l   # 100
```

### 8. Búsqueda por nombre

**Problema:** no se puede buscar. Para el buscador de «Mi equipo» el frontend descarga los 1351 Pokémon y filtra en el navegador.

**Propuesta:** `?search=texto`, sin distinguir mayúsculas, que busque dentro del `identifier` (y del nombre en español cuando exista la tarea 10). Debe funcionar junto con la paginación y con `total` (tarea 4).

**Cómo comprobarlo:**

```sh
curl -s "http://localhost:8080/api/v1/pokemon?search=char"   # charmander, charmeleon, charizard, charizard-mega-x...
curl -s "http://localhost:8080/api/v1/pokemon?search=zzzz"   # "pokemons":[] y "total":0
```

### 9. Filtros por tipo y por formas

**Problema:** el listado mezcla los 1025 Pokémon normales con 326 formas (megas, Pikachu disfrazado, etc.) y no se puede filtrar.

**Propuesta:**
- `?type=Fuego` (o el id del tipo): Pokémon que tienen ese tipo.
- `?default=true`: solo las formas por defecto (`is_default = 1`).

Se combinan entre sí y con `search` y la paginación.

**Cómo comprobarlo:**

```sh
curl -s "http://localhost:8080/api/v1/pokemon?default=true" | grep -o '"total":[0-9]*'   # "total":1025
curl -s "http://localhost:8080/api/v1/pokemon?type=Hada&page=1"                           # todos con Hada
```

### 10. Nombre en español para mostrar

**Problema:** solo hay `identifier` (`mr-mime`, `farfetchd`, `nidoran-f`). El frontend lo convierte en «Mr Mime», «Farfetchd» o «Nidoran F», que no son los nombres reales.

**Propuesta:** añadir `name` con el nombre oficial en español (`"Mr. Mime"`, `"Farfetch'd"`, `"Nidoran♀"`). Si los datos vienen del CSV de PokeAPI/veekun, están en `pokemon_species_names` (idioma español, `local_language_id = 7`). Para las formas (megas, etc.), `pokemon_form_names` tiene el nombre de la forma.

**Cómo comprobarlo:**

```sh
curl -s http://localhost:8080/api/v1/pokemon/122   # "name":"Mr. Mime"
```

### 11. Preparado para producción

**Problema:** en desarrollo el frontend llega a la API a través del proxy de Vite (`/api` → `localhost:8080`). En producción ese proxy no existe.

**Propuesta (una de las dos):**
- Servir el frontend y la API desde el mismo dominio, con la API bajo `/api` (p. ej. con un proxy inverso como nginx o Caddy). Es lo más sencillo y no necesita CORS.
- O habilitar CORS para el dominio del frontend (`Access-Control-Allow-Origin`) y decidir con qué variable se configura la URL de la API en el frontend.

Hoy no hay cabeceras CORS y la petición previa `OPTIONS` responde `404`.

**Cómo comprobarlo (opción CORS):**

```sh
curl -i -H "Origin: https://tu-dominio" http://localhost:8080/api/v1/pokemon   # Access-Control-Allow-Origin presente
```

### 12. Imagen de cada Pokémon

**Problema:** la API no devuelve ninguna imagen. Como solución provisional, el frontend construye la URL con el `id` contra los sprites públicos de PokeAPI:

```
https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/{id}.png
```

Funciona porque los ids de esta API coinciden con los de PokeAPI, también los de las formas (10034 es `charizard-mega-x` en las dos). Pero:
- El frontend depende de un repositorio de GitHub que no controlamos y de que los ids sigan coincidiendo.
- Algunas formas no tienen sprite (por ejemplo, el 10271 devuelve 404); en ese caso el frontend muestra la inicial.
- Las imágenes son propiedad de Nintendo/Game Freak; para un proyecto de fans sin ánimo de lucro es lo habitual, pero conviene tenerlo presente.

**Propuesta:** que cada Pokémon traiga sus URLs de imagen, y `null` cuando no exista:

```json
{"sprites": {"default": "https://.../sprites/pokemon/658.png",
             "artwork": "https://.../sprites/pokemon/other/official-artwork/658.png"}}
```

- `default`: el sprite pequeño (96×96, unos 1–2 KB) para listas.
- `artwork`: la ilustración grande (475×475) para fichas de detalle.

La API puede seguir apuntando a GitHub (lo importante es que la decisión y la comprobación de que existe estén en el backend) o servirlas ella misma descargándolas una vez, por ejemplo en `GET /api/v1/pokemon/:id/sprite`, lo que además evita depender de GitHub en producción.

**Cómo comprobarlo:**

```sh
curl -s http://localhost:8080/api/v1/pokemon/658     # "sprites":{"default":"...658.png",...}
curl -s http://localhost:8080/api/v1/pokemon/10271   # "sprites":{"default":null,...}
```

---

## Qué cambia en el frontend con cada tarea

Avísame cuando termines una y adapto esta parte:

| Tarea | Cambio en el frontend |
|---|---|
| 1 | `/estado` empieza a funcionar sin tocar nada |
| 2, 6 | ✅ hechas en la API. Falta en el frontend: `fetchPokemon` debe tratar el `404` como «no existe» (hoy lanza error, porque antes llegaba `200` vacío) y quitar el parche del cuerpo vacío en `src/api/pokemon.js` |
| 3 | quitar el parche de `pokemons: null` en `src/api/pokemon.js` |
| 4 | la Pokédex puede mostrar «N de 1351» |
| 5 | quitar el `split(',')` de los tipos |
| 7, 8 | el buscador de «Mi equipo» deja de descargar toda la Pokédex |
| 9 | filtros en la Pokédex |
| 10 | nombres correctos en todas las pantallas |
| 12 | usar `sprites` de la API y quitar la URL de GitHub de `src/api/pokemon.js` |
