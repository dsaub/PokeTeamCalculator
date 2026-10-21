package catalogo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrNoEncontrado = errors.New("no encontrado")

// Repo es la fuente de datos del catálogo. La implementación real lee de
// pokedata; las pruebas usan una implementación en memoria.
type Repo interface {
	Juegos(ctx context.Context) ([]Juego, error)
	Lideres(ctx context.Context, versionGroup string, gen int) ([]Lider, error)
	BuscarPokemon(ctx context.Context, gen int, q string, limite int) ([]PokemonResumen, error)
	Pokemon(ctx context.Context, gen int, especie string) (*PokemonDetalle, error)
}

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

const queryJuegos = `SELECT vg.identifier, vg.generation_id,
       GROUP_CONCAT(DISTINCT COALESCE(v.name_es, v.name_en, v.identifier) ORDER BY v.id SEPARATOR '|')
FROM version_groups vg
JOIN versions v ON v.version_group_id = vg.id
WHERE EXISTS (SELECT 1 FROM gym_battles gb
              WHERE gb.version_group_id = vg.id AND gb.battle_kind = 'gym')
GROUP BY vg.id, vg.identifier, vg.generation_id, vg.sort_order
ORDER BY vg.sort_order, vg.id`

func (r *MySQLRepository) Juegos(ctx context.Context) ([]Juego, error) {
	rows, err := r.db.QueryContext(ctx, queryJuegos)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var juegos []Juego
	vistos := make(map[string]int)
	for rows.Next() {
		var versionGroup string
		var gen int
		var nombres sql.NullString
		if err := rows.Scan(&versionGroup, &gen, &nombres); err != nil {
			return nil, err
		}
		lista := separarNombres(nombres.String)
		if len(lista) == 0 {
			lista = []string{versionGroup}
		}
		base := Slug(strings.Join(lista, "-"))
		if base == "" {
			base = Slug(versionGroup)
		}
		id := base
		if n := vistos[base]; n > 0 {
			id = fmt.Sprintf("%s-%d", base, n+1)
		}
		vistos[base]++
		juegos = append(juegos, Juego{
			ID:           id,
			Nombre:       NombreJuego(lista),
			Gen:          fmt.Sprintf("gen%d", gen),
			VersionGroup: versionGroup,
		})
	}
	return juegos, rows.Err()
}

const queryLideres = `SELECT gym_battle_id, generation, leader_en, leader_es,
       city_es, city_en, specialty_type_es, specialty_type, gym_order
FROM v_gym_battles
WHERE version_group = ? AND battle_kind = 'gym'
ORDER BY gym_order, gym_battle_id`

func (r *MySQLRepository) Lideres(ctx context.Context, versionGroup string, gen int) ([]Lider, error) {
	rows, err := r.db.QueryContext(ctx, queryLideres, versionGroup)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var lideres []Lider
	for rows.Next() {
		var l Lider
		var generacion int
		var leaderEn, leaderEs string
		var cityEs, cityEn, tipoEs, tipoEn sql.NullString
		if err := rows.Scan(&l.ID, &generacion, &leaderEn, &leaderEs, &cityEs, &cityEn, &tipoEs, &tipoEn, &l.Orden); err != nil {
			return nil, err
		}
		l.Nombre = primero(leaderEs, leaderEn)
		l.Ciudad = primero(cityEs.String, cityEn.String)
		tipo := primero(tipoEs.String, tipoEn.String)
		if tipo != "" {
			l.Tipo = &tipo
		}
		l.Equipo = []PokemonEquipo{}
		lideres = append(lideres, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(lideres) == 0 {
		return []Lider{}, nil
	}

	ids := make([]any, len(lideres))
	marcadores := make([]string, len(lideres))
	indice := make(map[int64]int, len(lideres))
	for i := range lideres {
		ids[i] = lideres[i].ID
		marcadores[i] = "?"
		indice[lideres[i].ID] = i
	}
	consulta := `SELECT t.gym_battle_id, t.generation, t.pokemon, t.pokemon_en, t.pokemon_es,
       t.level, t.ability_en, t.held_item_en, t.moves_en,
       vp.type1, vp.type2, pf.full_name_es, pf.full_name_en
FROM v_gym_teams t
JOIN pokemon p ON p.identifier = t.pokemon
LEFT JOIN pokemon_forms pf ON pf.pokemon_id = p.id AND pf.is_default = 1
LEFT JOIN v_pokemon vp ON vp.identifier = t.pokemon
WHERE t.gym_battle_id IN (` + strings.Join(marcadores, ",") + `)
ORDER BY t.gym_battle_id, t.slot`

	filas, err := r.db.QueryContext(ctx, consulta, ids...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()

	for filas.Next() {
		var battleID int64
		var generacion, nivel int
		var pokemon, pokemonEn, pokemonEs sql.NullString
		var habilidad, objeto, movimientos, type1, type2, fullEs, fullEn sql.NullString
		if err := filas.Scan(&battleID, &generacion, &pokemon, &pokemonEn, &pokemonEs,
			&nivel, &habilidad, &objeto, &movimientos, &type1, &type2, &fullEs, &fullEn); err != nil {
			return nil, err
		}
		pos, ok := indice[battleID]
		if !ok {
			continue
		}
		especie := pokemonEn.String
		if especie == "" {
			especie = pokemon.String
		}
		nombre := primero(fullEs.String, fullEn.String, pokemonEs.String, pokemonEn.String, especie)
		pk := PokemonEquipo{
			Especie:     especie,
			Nombre:      nombre,
			Nivel:       nivel,
			Movimientos: separarLista(movimientos.String),
			Tipos:       tipos(type1.String, type2.String),
		}
		if generacion >= 3 && habilidad.Valid && habilidad.String != "" {
			v := habilidad.String
			pk.Habilidad = &v
		}
		if generacion > 1 && objeto.Valid && objeto.String != "" {
			v := objeto.String
			pk.Objeto = &v
		}
		lideres[pos].Equipo = append(lideres[pos].Equipo, pk)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	return lideres, nil
}

const queryBuscar = `SELECT vp.identifier, vp.name_es, vp.name_en, vp.type1, vp.type2,
       p.is_default, s.identifier, s.name_en,
       pf.full_name_es, pf.full_name_en
FROM v_pokemon vp
JOIN pokemon p ON p.identifier = vp.identifier
JOIN pokemon_species s ON s.id = p.species_id
LEFT JOIN pokemon_forms pf ON pf.pokemon_id = p.id AND pf.is_default = 1
LEFT JOIN version_groups ivg ON ivg.id = pf.introduced_in_version_group_id
WHERE COALESCE(ivg.generation_id, s.generation_id) <= ?
  AND (vp.name_es LIKE ? ESCAPE '\\' OR vp.name_en LIKE ? ESCAPE '\\'
       OR vp.identifier LIKE ? ESCAPE '\\'
       OR pf.full_name_es LIKE ? ESCAPE '\\' OR pf.full_name_en LIKE ? ESCAPE '\\')
ORDER BY p.is_default DESC, s.id, p.sort_order, vp.identifier
LIMIT ?`

func (r *MySQLRepository) BuscarPokemon(ctx context.Context, gen int, q string, limite int) ([]PokemonResumen, error) {
	patron := "%" + escaparLike(q) + "%"
	rows, err := r.db.QueryContext(ctx, queryBuscar, gen, patron, patron, patron, patron, patron, limite)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	resumenes := []PokemonResumen{}
	for rows.Next() {
		var identifier, type1 sql.NullString
		var nameEs, nameEn, speciesIdent, speciesEn, fullEs, fullEn sql.NullString
		var type2 sql.NullString
		var isDefault int
		if err := rows.Scan(&identifier, &nameEs, &nameEn, &type1, &type2,
			&isDefault, &speciesIdent, &speciesEn, &fullEs, &fullEn); err != nil {
			return nil, err
		}
		baseEn := primero(speciesEn.String, nameEn.String, identifier.String)
		especie := NombreShowdown(identifier.String, speciesIdent.String, isDefault == 1, baseEn)
		nombre := primero(nameEs.String, nameEn.String, identifier.String)
		if isDefault == 0 {
			nombre = primero(fullEs.String, fullEn.String, nameEs.String, nameEn.String, identifier.String)
		}
		resumenes = append(resumenes, PokemonResumen{
			Especie: especie,
			Nombre:  nombre,
			Tipos:   tipos(type1.String, type2.String),
		})
	}
	return resumenes, rows.Err()
}

const queryPokemon = `SELECT p.id, p.identifier, p.is_default, s.identifier, s.name_en, s.name_es,
       vp.type1, vp.type2, pf.full_name_es, pf.full_name_en,
       COALESCE(ivg.generation_id, s.generation_id) AS gen_intro
FROM pokemon p
JOIN pokemon_species s ON s.id = p.species_id
LEFT JOIN pokemon_forms pf ON pf.pokemon_id = p.id AND pf.is_default = 1
LEFT JOIN version_groups ivg ON ivg.id = pf.introduced_in_version_group_id
LEFT JOIN v_pokemon vp ON vp.identifier = p.identifier
WHERE LOWER(REPLACE(p.identifier, '-', '')) = ?
LIMIT 1`

func (r *MySQLRepository) Pokemon(ctx context.Context, gen int, especie string) (*PokemonDetalle, error) {
	norm := normalizarEspecie(especie)
	if norm == "" {
		return nil, ErrNoEncontrado
	}

	var pokemonID int64
	var identifier, speciesIdent, nameEn, nameEs sql.NullString
	var type1, type2, fullEs, fullEn sql.NullString
	var isDefault, genIntro int
	err := r.db.QueryRowContext(ctx, queryPokemon, norm).Scan(
		&pokemonID, &identifier, &isDefault, &speciesIdent, &nameEn, &nameEs,
		&type1, &type2, &fullEs, &fullEn, &genIntro)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, err
	}
	if genIntro > gen {
		return nil, ErrNoEncontrado
	}

	baseEn := primero(nameEn.String, identifier.String)
	detalle := &PokemonDetalle{
		Especie:     NombreShowdown(identifier.String, speciesIdent.String, isDefault == 1, baseEn),
		Tipos:       tipos(type1.String, type2.String),
		Habilidades: []Habilidad{},
		Movimientos: []Movimiento{},
	}
	if isDefault == 1 {
		detalle.Nombre = primero(nameEs.String, nameEn.String, identifier.String)
	} else {
		detalle.Nombre = primero(fullEs.String, fullEn.String, nameEs.String, nameEn.String, identifier.String)
	}

	if gen >= 3 {
		habilidades, err := r.habilidades(ctx, pokemonID, gen)
		if err != nil {
			return nil, err
		}
		detalle.Habilidades = habilidades
	}
	movimientos, err := r.movimientos(ctx, pokemonID, gen)
	if err != nil {
		return nil, err
	}
	detalle.Movimientos = movimientos
	return detalle, nil
}

const queryHabilidades = `SELECT pa.slot, pa.is_hidden, pa.ability_id, a.generation_id,
       a.identifier, a.name_en, a.name_es, 0
FROM pokemon_abilities pa
JOIN abilities a ON a.id = pa.ability_id
WHERE pa.pokemon_id = ?
UNION ALL
SELECT pap.slot, pap.is_hidden, COALESCE(pap.ability_id, 0), COALESCE(a.generation_id, 0),
       COALESCE(a.identifier, ''), COALESCE(a.name_en, ''), COALESCE(a.name_es, ''), pap.generation_id
FROM pokemon_abilities_past pap
LEFT JOIN abilities a ON a.id = pap.ability_id
WHERE pap.pokemon_id = ?`

type filaHabilidad struct {
	slot       int
	hidden     bool
	abilityID  int
	generacion int
	identifier string
	nameEn     string
	nameEs     string
	pasadaGen  int
}

func (r *MySQLRepository) habilidades(ctx context.Context, pokemonID int64, gen int) ([]Habilidad, error) {
	rows, err := r.db.QueryContext(ctx, queryHabilidades, pokemonID, pokemonID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var filas []filaHabilidad
	for rows.Next() {
		var f filaHabilidad
		if err := rows.Scan(&f.slot, &f.hidden, &f.abilityID, &f.generacion,
			&f.identifier, &f.nameEn, &f.nameEs, &f.pasadaGen); err != nil {
			return nil, err
		}
		filas = append(filas, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	porSlot := make(map[int]filaHabilidad)
	for _, f := range filas {
		if f.pasadaGen == 0 {
			porSlot[f.slot] = f
		}
	}
	mejor := make(map[int]filaHabilidad)
	for _, f := range filas {
		if f.pasadaGen == 0 || f.pasadaGen < gen {
			continue
		}
		if actual, ok := mejor[f.slot]; !ok || f.pasadaGen < actual.pasadaGen {
			mejor[f.slot] = f
		}
	}
	for slot, f := range mejor {
		porSlot[slot] = f
	}

	slots := make([]int, 0, len(porSlot))
	for slot := range porSlot {
		slots = append(slots, slot)
	}
	sortInts(slots)

	var salida []Habilidad
	vistos := make(map[string]bool)
	for _, slot := range slots {
		f := porSlot[slot]
		if f.abilityID == 0 || f.generacion > gen {
			continue
		}
		id := primero(f.nameEn, f.identifier)
		if id == "" || vistos[id] {
			continue
		}
		vistos[id] = true
		salida = append(salida, Habilidad{
			ID:     id,
			Nombre: primero(f.nameEs, f.nameEn, f.identifier),
		})
	}
	if salida == nil {
		salida = []Habilidad{}
	}
	return salida, nil
}

const queryMovimientos = `SELECT m.identifier, m.name_en, m.name_es, t.identifier,
       dc.identifier, m.power
FROM pokemon_moves pm
JOIN version_groups vg ON vg.id = pm.version_group_id
JOIN moves m ON m.id = pm.move_id
JOIN types t ON t.id = m.type_id
LEFT JOIN move_damage_classes dc ON dc.id = m.damage_class_id
WHERE pm.pokemon_id = ? AND vg.generation_id = ?
GROUP BY m.id, m.identifier, m.name_en, m.name_es, t.identifier, dc.identifier, m.power
ORDER BY m.name_en, m.identifier`

func (r *MySQLRepository) movimientos(ctx context.Context, pokemonID int64, gen int) ([]Movimiento, error) {
	rows, err := r.db.QueryContext(ctx, queryMovimientos, pokemonID, gen)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	salida := []Movimiento{}
	for rows.Next() {
		var identifier, nameEn, nameEs, tipo, categoria sql.NullString
		var potencia sql.NullInt64
		if err := rows.Scan(&identifier, &nameEn, &nameEs, &tipo, &categoria, &potencia); err != nil {
			return nil, err
		}
		m := Movimiento{
			ID:     primero(nameEn.String, identifier.String),
			Nombre: primero(nameEs.String, nameEn.String, identifier.String),
			Tipo:   tipo.String,
		}
		if categoria.Valid && categoria.String != "" {
			v := categoria.String
			m.Categoria = &v
		}
		if potencia.Valid {
			v := int(potencia.Int64)
			m.Potencia = &v
		}
		salida = append(salida, m)
	}
	return salida, rows.Err()
}

func separarNombres(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	partes := strings.Split(s, "|")
	salida := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != "" {
			salida = append(salida, p)
		}
	}
	return salida
}

func separarLista(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	partes := strings.Split(s, ", ")
	salida := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != "" {
			salida = append(salida, p)
		}
	}
	return salida
}

func tipos(a, b string) []string {
	salida := []string{}
	if a != "" {
		salida = append(salida, a)
	}
	if b != "" {
		salida = append(salida, b)
	}
	return salida
}

func primero(valores ...string) string {
	for _, v := range valores {
		if v != "" {
			return v
		}
	}
	return ""
}

func escaparLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func normalizarEspecie(s string) string {
	s = reemplazosNombre.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
