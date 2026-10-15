-- name: GetAllPokemons :many
SELECT 
  pokemon.id AS id,
  pokemon.identifier AS identifier,
  pokemon.species_id AS species_id,
  pokemon.height_dm AS height,
  pokemon.weight_hg AS weight,
  pokemon.base_experience AS base_experience,
  pokemon.sort_order AS sort_order,
  pokemon.is_default AS is_default,
  GROUP_CONCAT(types.id ORDER BY pokemon_types.slot SEPARATOR ', ') as type_ids,
  GROUP_CONCAT(types.name_es ORDER BY pokemon_types.slot SEPARATOR ', ') as type_names
FROM 
  pokemon
INNER JOIN pokemon_types ON pokemon.id = pokemon_types.pokemon_id
INNER JOIN types ON pokemon_types.type_id = types.id
GROUP BY pokemon.id;

-- name: GetPokemonByID :one
SELECT
    pokemon.id AS id,
    pokemon.identifier AS identifier,
    pokemon.species_id AS species_id,
    pokemon.height_dm AS height,
    pokemon.weight_hg AS weight,
    pokemon.base_experience AS base_experience,
    pokemon.sort_order AS sort_order,
    pokemon.is_default AS is_default,
    GROUP_CONCAT(types.id ORDER BY pokemon_types.slot SEPARATOR ', ') as type_ids,
    GROUP_CONCAT(types.name_es ORDER BY pokemon_types.slot SEPARATOR ', ') as type_names
FROM
    pokemon

INNER JOIN pokemon_types ON pokemon.id = pokemon_types.pokemon_id
INNER JOIN types ON pokemon_types.type_id = types.id
WHERE pokemon.id = ?
GROUP BY pokemon.id;

-- name: GetPokemonQuantity :one
SELECT
    COUNT(pokemon.id) AS pokemons
FROM
    pokemon;

-- name: GetPokemonsPager :many
SELECT
    pokemon.id AS id,
    pokemon.identifier AS identifier,
    pokemon.species_id AS species_id,
    pokemon.height_dm AS height,
    pokemon.weight_hg AS weight,
    pokemon.base_experience AS base_experience,
    pokemon.sort_order AS sort_order,
    pokemon.is_default AS is_default,
    GROUP_CONCAT(types.id ORDER BY pokemon_types.slot SEPARATOR ', ') as type_ids,
    GROUP_CONCAT(types.name_es ORDER BY pokemon_types.slot SEPARATOR ', ') as type_names
FROM
    pokemon
        INNER JOIN pokemon_types ON pokemon.id = pokemon_types.pokemon_id
        INNER JOIN types ON pokemon_types.type_id = types.id
GROUP BY pokemon.id
LIMIT 20
OFFSET ?;