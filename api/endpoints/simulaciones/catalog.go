package simulaciones

import (
	"context"
	"database/sql"
)

// Catalog permite comprobar contra pokedata si una habilidad pertenece a una
// especie en una generación concreta. Es una comprobación de apoyo: el worker
// sigue siendo la autoridad final con los datos de Showdown.
type Catalog interface {
	HabilidadValida(ctx context.Context, gen int, especie, habilidad string) (existeEspecie bool, valida bool, err error)
}

type MySQLCatalog struct {
	db *sql.DB
}

func NewMySQLCatalog(db *sql.DB) *MySQLCatalog {
	return &MySQLCatalog{db: db}
}

type filaHabilidad struct {
	slot       int
	hidden     bool
	abilityID  int
	generacion int
	ident      string
	pasadaGen  int // 0 = habilidad actual
}

// HabilidadValida replica la resolución de habilidades por generación de
// pokedata: parte de las actuales y aplica los registros "past" cuyo
// generation_id es mayor o igual que la generación pedida, quedándose con el
// más cercano por slot.
func (c *MySQLCatalog) HabilidadValida(ctx context.Context, gen int, especie, habilidad string) (bool, bool, error) {
	normEspecie := toID(especie)
	if normEspecie == "" {
		return false, false, nil
	}

	var id int
	err := c.db.QueryRowContext(ctx,
		`SELECT id FROM pokemon WHERE LOWER(REPLACE(identifier, '-', '')) = ? LIMIT 1`,
		normEspecie).Scan(&id)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}

	rows, err := c.db.QueryContext(ctx, `
SELECT pa.slot, pa.is_hidden, pa.ability_id, a.generation_id, a.identifier, 0
FROM pokemon_abilities pa
JOIN abilities a ON a.id = pa.ability_id
WHERE pa.pokemon_id = ?
UNION ALL
SELECT pap.slot, pap.is_hidden, COALESCE(pap.ability_id, 0), COALESCE(a.generation_id, 0),
       COALESCE(a.identifier, ''), pap.generation_id
FROM pokemon_abilities_past pap
LEFT JOIN abilities a ON a.id = pap.ability_id
WHERE pap.pokemon_id = ?`, id, id)
	if err != nil {
		return true, false, err
	}
	defer func() { _ = rows.Close() }()

	var filas []filaHabilidad
	for rows.Next() {
		var f filaHabilidad
		if err := rows.Scan(&f.slot, &f.hidden, &f.abilityID, &f.generacion, &f.ident, &f.pasadaGen); err != nil {
			return true, false, err
		}
		filas = append(filas, f)
	}
	if err := rows.Err(); err != nil {
		return true, false, err
	}

	return true, habilidadEnFilas(filas, gen, habilidad), nil
}

func habilidadEnFilas(filas []filaHabilidad, gen int, habilidad string) bool {
	// Habilidades actuales por slot (incluidas las ocultas: Showdown las
	// admite si existen en esa generación).
	porSlot := make(map[int]filaHabilidad)
	for _, f := range filas {
		if f.pasadaGen == 0 {
			porSlot[f.slot] = f
		}
	}
	// Overrides "past": el registro con la generación más cercana >= gen.
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

	objetivo := toID(habilidad)
	for _, f := range porSlot {
		if f.abilityID == 0 || f.generacion > gen {
			continue
		}
		if toID(f.ident) == objetivo {
			return true
		}
	}
	return false
}
