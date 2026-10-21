-- Corrige v_gym_teams.pokemon_en / pokemon_es para que conserven la forma.
--
-- Requiere que pokedata haya creado sus tablas y vistas (v_gym_battles,
-- gym_battle_pokemon, pokemon, pokemon_species...). Si la base todavía no
-- tiene esos datos, se puede aplicar después de la importación de pokedata.
--
-- pokedata usaba s.name_en (nombre de la especie), así que "Yamask-Galar"
-- llegaba como "Yamask", "Weezing-Galar" como "Weezing", etc. Esta vista
-- reconstruye el nombre de Showdown a partir de p.identifier:
--   yamask-galar                -> Yamask-Galar
--   weezing-galar               -> Weezing-Galar
--   toxtricity-low-key          -> Toxtricity-Low-Key
--   meowstic-female             -> Meowstic-Female (Showdown lo canoniza a Meowstic-F)
--   darmanitan-galar-standard   -> Darmanitan-Galar
-- Las formas por defecto siguen usando el nombre de la especie.
--
-- Nota: pokedata recrea sus vistas al importar, así que esta migración debe
-- volver a aplicarse después de cada importación de pokedata.

CREATE OR REPLACE VIEW v_gym_teams AS
SELECT b.gym_battle_id, b.generation, b.version_group, b.version, b.region,
       b.city_en, b.gym_order, b.leader_en, b.leader_es, b.specialty_type,
       b.battle_kind, b.rematch_number,
       bp.slot,
       p.identifier      AS pokemon,
       CASE WHEN f.forma = '' THEN f.nombre_base_es
            ELSE CONCAT(f.nombre_base_es, '-', f.forma_display) END AS pokemon_es,
       CASE WHEN f.forma = '' THEN f.nombre_base_en
            ELSE CONCAT(f.nombre_base_en, '-', f.forma_display) END AS pokemon_en,
       bp.level,
       bp.gender,
       a.name_es         AS ability_es,
       a.name_en         AS ability_en,
       bp.ability_source,
       i.name_es         AS held_item_es,
       i.name_en         AS held_item_en,
       tt.identifier     AS tera_type,
       bp.can_dynamax,
       bp.is_gigantamax,
       (SELECT GROUP_CONCAT(m.name_es ORDER BY bm.slot ASC SEPARATOR ', ')
        FROM gym_battle_pokemon_moves bm
        JOIN moves m ON m.id = bm.move_id
        WHERE bm.gym_battle_pokemon_id = bp.id) AS moves_es,
       (SELECT GROUP_CONCAT(m.name_en ORDER BY bm.slot ASC SEPARATOR ', ')
        FROM gym_battle_pokemon_moves bm
        JOIN moves m ON m.id = bm.move_id
        WHERE bm.gym_battle_pokemon_id = bp.id) AS moves_en,
       bp.moves_source
FROM v_gym_battles b
JOIN gym_battle_pokemon bp ON bp.gym_battle_id = b.gym_battle_id
JOIN pokemon p             ON p.id = bp.pokemon_id
JOIN pokemon_species s     ON s.id = p.species_id
JOIN (
    SELECT id, nombre_base_en, nombre_base_es, forma,
           CONCAT(
             UCASE(LEFT(SUBSTRING_INDEX(forma, '-', 1), 1)),
             SUBSTRING(SUBSTRING_INDEX(forma, '-', 1), 2),
             IF(forma LIKE '%-%', CONCAT('-',
                UCASE(LEFT(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 2), '-', -1), 1)),
                SUBSTRING(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 2), '-', -1), 2)), ''),
             IF((LENGTH(forma) - LENGTH(REPLACE(forma, '-', ''))) >= 2, CONCAT('-',
                UCASE(LEFT(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 3), '-', -1), 1)),
                SUBSTRING(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 3), '-', -1), 2)), ''),
             IF((LENGTH(forma) - LENGTH(REPLACE(forma, '-', ''))) >= 3, CONCAT('-',
                UCASE(LEFT(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 4), '-', -1), 1)),
                SUBSTRING(SUBSTRING_INDEX(SUBSTRING_INDEX(forma, '-', 4), '-', -1), 2)), '')
           ) AS forma_display
    FROM (
        SELECT p2.id,
               s2.name_en AS nombre_base_en,
               s2.name_es AS nombre_base_es,
               CASE
                 WHEN p2.is_default = 1 OR p2.identifier = s2.identifier THEN ''
                 ELSE REPLACE(SUBSTRING(p2.identifier, CHAR_LENGTH(s2.identifier) + 2), '-standard', '')
               END AS forma
        FROM pokemon p2
        JOIN pokemon_species s2 ON s2.id = p2.species_id
    ) base
) f ON f.id = p.id
LEFT JOIN abilities a      ON a.id = bp.ability_id
LEFT JOIN items i          ON i.id = bp.held_item_id
LEFT JOIN types tt         ON tt.id = bp.tera_type_id;

-- Diagnóstico de formas afectadas por el bug (5 filas / 5 formas con los
-- datos actuales de pokedata):
--   SELECT COUNT(*) AS filas, COUNT(DISTINCT p.id) AS formas
--   FROM v_gym_teams t
--   JOIN pokemon p ON p.identifier = t.pokemon
--   JOIN pokemon_species s ON s.id = p.species_id
--   WHERE p.is_default = 0 AND p.identifier <> s.identifier;
