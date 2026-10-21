# Catálogo

Endpoints de solo lectura para que la web construya la petición de simulación.
Las respuestas correctas llevan `Cache-Control: public, max-age=3600` porque los
datos de pokedata no cambian mientras no se reimporte la base. Los errores no se
cachean.

Los nombres `especie`, `movimientos`, `habilidad` y `objeto` van en inglés y en
formato Showdown (lo que acepta el worker). `nombre` es para mostrar: español
si pokedata lo tiene; si no, inglés.

## `GET /api/juegos`

```json
[
  { "id": "platino", "nombre": "Pokémon Platino", "gen": "gen4" }
]
```

Solo aparecen los juegos con gimnasios. El `id` se deriva de los nombres
españoles de las versiones (`Platino` → `platino`, `Diamante` + `Perla` →
`diamante-perla`). Es estable mientras no cambie pokedata.

## `GET /api/juegos/{juego}/lideres`

Devuelve los combates de gimnasio principales (`battle_kind = 'gym'`) en orden
de medalla. Los rematches y combates especiales no se incluyen para no repetir
líderes.

```json
[
  {
    "id": 123,
    "nombre": "Roco",
    "ciudad": "Oreburgh City",
    "tipo": "Roca",
    "orden": 1,
    "equipo": [
      {
        "especie": "Cranidos",
        "nombre": "Cranidos",
        "nivel": 14,
        "habilidad": "Mold Breaker",
        "objeto": null,
        "movimientos": ["Headbutt", "Pursuit", "Leer"],
        "tipos": ["rock"]
      }
    ]
  }
]
```

- `id` es el `gym_battle_id`.
- `habilidad` es `null` en gen 1–2 y `objeto` en gen 1.
- `tipos` son identificadores ingleses (`rock`, `ground`, …).
- Las formas regionales llevan la forma en `especie` (`Weezing-Galar`).
- En juegos con líderes exclusivos de versión (por ejemplo, Espada/Escudo)
  aparecen los dos, con el mismo `orden`, y `id` distinto.
- Si el juego no existe → `404`.

## `GET /api/pokemon?gen=gen4&q=monf`

Autocompletado por nombre español, inglés o identificador. Máximo 20
resultados. `q` debe tener al menos 2 caracteres y `gen` es obligatorio; si no,
`400`.

```json
[
  { "especie": "Monferno", "nombre": "Monferno", "tipos": ["fire", "fighting"] }
]
```

Solo se incluyen especies y formas introducidas en esa generación o antes.

## `GET /api/pokemon/{especie}?gen=gen4`

```json
{
  "especie": "Monferno",
  "nombre": "Monferno",
  "tipos": ["fire", "fighting"],
  "habilidades": [{ "id": "Blaze", "nombre": "Mar Llamas" }],
  "movimientos": [
    {
      "id": "Mach Punch",
      "nombre": "Puño Máquina",
      "tipo": "fighting",
      "categoria": "physical",
      "potencia": 40
    }
  ]
}
```

- `movimientos` es la unión de los aprendibles en cualquier juego de esa
  generación (`pokemon_moves` + `version_groups.generation_id`).
- `habilidades` está vacío en gen 1–2.
- `categoria` es `physical`, `special` o `status`; `potencia` es `null` en
  movimientos de estado.
- Si la especie o forma no existe en esa generación → `404`.

## Origen de los datos

Todo sale de pokedata:

| Dato | Tablas/vistas |
|---|---|
| Juegos | `version_groups`, `versions`, `gym_battles` |
| Líderes y ciudades | `v_gym_battles` |
| Equipos, habilidades, objetos, movimientos | `v_gym_teams` |
| Formas en formato Showdown | `pokemon`, `pokemon_species`, `pokemon_forms` |
| Tipos | `v_pokemon` |
| Habilidades por generación | `pokemon_abilities`, `pokemon_abilities_past`, `abilities` |
| Movimientos por generación | `pokemon_moves`, `version_groups`, `moves`, `move_damage_classes` |
| Nombres en español | `name_es` de especies, movimientos, habilidades, objetos, tipos y localizaciones |

No ha faltado ningún dato para los endpoints. Dos matices:

1. Los nombres de juego no están en una tabla única: se componen con
   `versions.name_es` (por ejemplo, «Diamante» + «Perla» → «Pokémon Diamante y
   Perla»). Es una decisión de presentación, no un dato inventado.
2. `pokemon_forms.full_name_es` suele ser `NULL` para las formas regionales. En
   ese caso se usa `full_name_en` («Galarian Weezing») y, si tampoco existe, el
   nombre español de la especie con el sufijo de forma. Las ciudades usan
   `locations.name_es` y caen a `city_en` cuando pokedata no tiene traducción
   (por ejemplo, «Oreburgh City»).

## Pruebas

```bash
cd api
go test ./...
```

Las pruebas de catálogo contra MariaDB se omiten si `DATABASE_DSN_TEST` no
está definida o si esa base no tiene `v_gym_teams`. Para probarlas con una
copia de pokedata:

```bash
DATABASE_DSN_TEST='usuario:clave@tcp(127.0.0.1:3306)/pokedata?parseTime=true' go test ./...
```

Las pruebas viven en `api/`; ejecuta los comandos desde ese directorio.
```
