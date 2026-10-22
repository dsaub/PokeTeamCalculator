package catalogo

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Repositorio MySQL contra sqlmock: comprueba cómo se leen y combinan las
// filas, sin MariaDB. Las consultas reales se prueban en repository_mysql_test.go.

func repoFalsoSQL(t *testing.T) (*MySQLRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return NewMySQLRepository(db), mock
}

func igual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\nobtenido: %#v\nesperado: %#v", got, want)
	}
}

func ptr[T any](v T) *T { return &v }

var errBase = errors.New("base caída")

func TestRepoJuegos(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM version_groups vg`).WillReturnRows(sqlmock.NewRows([]string{"vg", "gen", "nombres"}).
		AddRow("diamond-pearl", 4, "Diamante|Perla").
		AddRow("platinum", 4, "Platino").
		AddRow("platinum-bis", 4, "Platino").
		AddRow("sin-nombres", 3, nil).
		AddRow("x-y", 6, " | "))
	juegos, err := r.Juegos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	igual(t, juegos, []Juego{
		{ID: "diamante-perla", Nombre: "Pokémon Diamante y Perla", Gen: "gen4", VersionGroup: "diamond-pearl"},
		{ID: "platino", Nombre: "Pokémon Platino", Gen: "gen4", VersionGroup: "platinum"},
		{ID: "platino-2", Nombre: "Pokémon Platino", Gen: "gen4", VersionGroup: "platinum-bis"},
		{ID: "sin-nombres", Nombre: "Pokémon sin-nombres", Gen: "gen3", VersionGroup: "sin-nombres"},
		{ID: "x-y", Nombre: "Pokémon x-y", Gen: "gen6", VersionGroup: "x-y"},
	})
}

func TestRepoJuegosErrores(t *testing.T) {
	t.Run("consulta", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM version_groups`).WillReturnError(errBase)
		if _, err := r.Juegos(context.Background()); !errors.Is(err, errBase) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("fila con otro tipo", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM version_groups`).WillReturnRows(sqlmock.NewRows([]string{"vg", "gen", "nombres"}).AddRow("x", "no-num", "X"))
		if _, err := r.Juegos(context.Background()); err == nil {
			t.Fatal("se esperaba un error de Scan")
		}
	})
	t.Run("error al recorrer", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM version_groups`).WillReturnRows(sqlmock.NewRows([]string{"vg", "gen", "nombres"}).
			AddRow("platinum", 4, "Platino").RowError(0, errBase))
		if _, err := r.Juegos(context.Background()); !errors.Is(err, errBase) {
			t.Fatalf("error %v", err)
		}
	})
}

var columnasLider = []string{"id", "gen", "leader_en", "leader_es", "city_es", "city_en", "tipo_es", "tipo_en", "orden"}
var columnasEquipo = []string{"id", "gen", "pokemon", "pokemon_en", "pokemon_es", "nivel", "habilidad", "objeto", "movs", "t1", "t2", "full_es", "full_en"}

func TestRepoLideres(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM v_gym_battles`).WithArgs("platinum").WillReturnRows(sqlmock.NewRows(columnasLider).
		AddRow(10, 4, "Roark", "Roco", "Pirita", "Oreburgh", "Roca", "rock", 1).
		AddRow(11, 4, "Gardenia", "", nil, "Eterna", nil, "grass", 2).
		AddRow(12, 4, "Sin equipo", "", nil, nil, nil, nil, 3))
	mock.ExpectQuery(`FROM v_gym_teams t`).WithArgs(10, 11, 12).WillReturnRows(sqlmock.NewRows(columnasEquipo).
		AddRow(10, 4, "geodude", "Geodude", "Geodude", 12, "Sturdy", "", "Tackle, Rock Throw", "rock", "ground", nil, nil).
		AddRow(10, 4, "cranidos", "Cranidos", "Cranidos", 14, "Mold Breaker", "Oran Berry", "Headbutt, Pursuit, Leer", "rock", nil, "Cranidos", nil).
		AddRow(11, 4, "roserade", nil, nil, 22, nil, nil, "", "grass", "poison", nil, nil).
		AddRow(99, 4, "huerfano", "Huerfano", nil, 1, nil, nil, "Tackle", nil, nil, nil, nil))
	lideres, err := r.Lideres(context.Background(), "platinum", 4)
	if err != nil {
		t.Fatal(err)
	}
	igual(t, lideres, []Lider{
		{ID: 10, Nombre: "Roco", Ciudad: "Pirita", Tipo: ptr("Roca"), Orden: 1, Equipo: []PokemonEquipo{
			{Especie: "Geodude", Nombre: "Geodude", Nivel: 12, Habilidad: ptr("Sturdy"), Movimientos: []string{"Tackle", "Rock Throw"}, Tipos: []string{"rock", "ground"}},
			{Especie: "Cranidos", Nombre: "Cranidos", Nivel: 14, Habilidad: ptr("Mold Breaker"), Objeto: ptr("Oran Berry"), Movimientos: []string{"Headbutt", "Pursuit", "Leer"}, Tipos: []string{"rock"}},
		}},
		{ID: 11, Nombre: "Gardenia", Ciudad: "Eterna", Tipo: ptr("grass"), Orden: 2, Equipo: []PokemonEquipo{
			{Especie: "roserade", Nombre: "roserade", Nivel: 22, Movimientos: []string{}, Tipos: []string{"grass", "poison"}},
		}},
		{ID: 12, Nombre: "Sin equipo", Ciudad: "", Orden: 3, Equipo: []PokemonEquipo{}},
	})
}

func TestRepoLideresReglasDeGeneracion(t *testing.T) {
	// En gen 1 no hay objetos ni habilidades; en gen 2, objetos pero no habilidades.
	for _, caso := range []struct {
		gen               int
		habilidad, objeto bool
	}{{1, false, false}, {2, false, true}, {3, true, true}} {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM v_gym_battles`).WillReturnRows(sqlmock.NewRows(columnasLider).AddRow(1, caso.gen, "Brock", "Brock", "Plateada", nil, nil, nil, 1))
		mock.ExpectQuery(`FROM v_gym_teams`).WillReturnRows(sqlmock.NewRows(columnasEquipo).
			AddRow(1, caso.gen, "onix", "Onix", "Onix", 14, "Sturdy", "Leftovers", "Tackle", "rock", "ground", nil, nil))
		lideres, err := r.Lideres(context.Background(), "red-blue", caso.gen)
		if err != nil {
			t.Fatal(err)
		}
		pk := lideres[0].Equipo[0]
		if (pk.Habilidad != nil) != caso.habilidad || (pk.Objeto != nil) != caso.objeto {
			t.Fatalf("gen %d: habilidad %v, objeto %v", caso.gen, pk.Habilidad, pk.Objeto)
		}
	}
}

func TestRepoLideresSinLideresNoConsultaEquipos(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM v_gym_battles`).WillReturnRows(sqlmock.NewRows(columnasLider))
	lideres, err := r.Lideres(context.Background(), "nada", 4)
	if err != nil {
		t.Fatal(err)
	}
	if lideres == nil || len(lideres) != 0 {
		t.Fatalf("lideres %#v", lideres)
	}
}

func TestRepoLideresErrores(t *testing.T) {
	unLider := func() *sqlmock.Rows {
		return sqlmock.NewRows(columnasLider).AddRow(1, 4, "Roark", "Roco", nil, nil, nil, nil, 1)
	}
	casos := []struct {
		nombre   string
		preparar func(sqlmock.Sqlmock)
	}{
		{"líderes", func(m sqlmock.Sqlmock) { m.ExpectQuery(`v_gym_battles`).WillReturnError(errBase) }},
		{"scan de líder", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`v_gym_battles`).WillReturnRows(sqlmock.NewRows(columnasLider).AddRow("x", 4, "a", "b", nil, nil, nil, nil, 1))
		}},
		{"recorrer líderes", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`v_gym_battles`).WillReturnRows(unLider().RowError(0, errBase))
		}},
		{"equipos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`v_gym_battles`).WillReturnRows(unLider())
			m.ExpectQuery(`v_gym_teams`).WillReturnError(errBase)
		}},
		{"scan de equipo", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`v_gym_battles`).WillReturnRows(unLider())
			m.ExpectQuery(`v_gym_teams`).WillReturnRows(sqlmock.NewRows(columnasEquipo).AddRow(1, 4, "a", "A", "A", "x", nil, nil, "", nil, nil, nil, nil))
		}},
		{"recorrer equipos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`v_gym_battles`).WillReturnRows(unLider())
			m.ExpectQuery(`v_gym_teams`).WillReturnRows(sqlmock.NewRows(columnasEquipo).
				AddRow(1, 4, "a", "A", "A", 5, nil, nil, "", nil, nil, nil, nil).RowError(0, errBase))
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			r, mock := repoFalsoSQL(t)
			caso.preparar(mock)
			if _, err := r.Lideres(context.Background(), "platinum", 4); err == nil {
				t.Fatal("se esperaba un error")
			}
		})
	}
}

var columnasBuscar = []string{"identifier", "name_es", "name_en", "t1", "t2", "is_default", "s_ident", "s_en", "full_es", "full_en"}

func TestRepoBuscarPokemon(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	patron := `%50\%\_a\\b%`
	mock.ExpectQuery(`FROM v_pokemon vp`).WithArgs(8, patron, patron, patron, patron, patron, 20).
		WillReturnRows(sqlmock.NewRows(columnasBuscar).
			AddRow("yamask", "Yamask", "Yamask", "ghost", nil, 1, "yamask", "Yamask", nil, nil).
			AddRow("yamask-galar", "Yamask", "Yamask", "ground", "ghost", 0, "yamask", "Yamask", "Yamask de Galar", nil).
			AddRow("raro", nil, nil, nil, nil, 1, "raro", nil, nil, nil))
	res, err := r.BuscarPokemon(context.Background(), 8, `50%_a\b`, 20)
	if err != nil {
		t.Fatal(err)
	}
	igual(t, res, []PokemonResumen{
		{Especie: "Yamask", Nombre: "Yamask", Tipos: []string{"ghost"}},
		{Especie: "Yamask-Galar", Nombre: "Yamask de Galar", Tipos: []string{"ground", "ghost"}},
		{Especie: "raro", Nombre: "raro", Tipos: []string{}},
	})
}

func TestRepoBuscarPokemonErrores(t *testing.T) {
	t.Run("consulta", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM v_pokemon`).WillReturnError(errBase)
		if _, err := r.BuscarPokemon(context.Background(), 4, "pi", 20); !errors.Is(err, errBase) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("scan", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM v_pokemon`).WillReturnRows(sqlmock.NewRows(columnasBuscar).AddRow("a", nil, nil, nil, nil, "x", nil, nil, nil, nil))
		if _, err := r.BuscarPokemon(context.Background(), 4, "pi", 20); err == nil {
			t.Fatal("se esperaba un error")
		}
	})
	t.Run("sin resultados → lista vacía", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM v_pokemon`).WillReturnRows(sqlmock.NewRows(columnasBuscar))
		res, err := r.BuscarPokemon(context.Background(), 4, "zz", 20)
		if err != nil || res == nil || len(res) != 0 {
			t.Fatalf("(%#v, %v)", res, err)
		}
	})
}

var columnasPokemon = []string{"id", "identifier", "is_default", "s_ident", "name_en", "name_es", "t1", "t2", "full_es", "full_en", "gen_intro"}
var columnasHabilidades = []string{"slot", "hidden", "ability_id", "gen", "identifier", "name_en", "name_es", "pasada"}
var columnasMovimientos = []string{"identifier", "name_en", "name_es", "tipo", "categoria", "potencia"}

func TestRepoPokemon(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM pokemon p`).WithArgs("mrmime").WillReturnRows(sqlmock.NewRows(columnasPokemon).
		AddRow(122, "mr-mime", 1, "mr-mime", "Mr. Mime", "Mr. Mime", "psychic", "fairy", nil, nil, 1))
	mock.ExpectQuery(`FROM pokemon_abilities pa`).WithArgs(122, 122).WillReturnRows(sqlmock.NewRows(columnasHabilidades).
		AddRow(1, false, 43, 3, "soundproof", "Soundproof", "Insonorizar", 0).
		AddRow(2, false, 111, 4, "filter", "Filter", "Filtro", 0).
		AddRow(3, true, 101, 4, "technician", "Technician", "Experto", 0).
		// Habilidad oculta posterior a gen 4: no cuenta.
		AddRow(4, true, 300, 9, "nueva", "Nueva", "", 0).
		// Duplicada: solo una vez.
		AddRow(5, true, 111, 4, "filter", "Filter", "Filtro", 0))
	mock.ExpectQuery(`FROM pokemon_moves pm`).WithArgs(122, 4).WillReturnRows(sqlmock.NewRows(columnasMovimientos).
		AddRow("confusion", "Confusion", "Confusión", "psychic", "special", 50).
		AddRow("barrier", "Barrier", nil, "psychic", "status", nil).
		AddRow("raro", nil, nil, "normal", nil, nil))
	d, err := r.Pokemon(context.Background(), 4, " Mr. Mime ")
	if err != nil {
		t.Fatal(err)
	}
	igual(t, d, &PokemonDetalle{
		Especie: "Mr. Mime", Nombre: "Mr. Mime", Tipos: []string{"psychic", "fairy"},
		Habilidades: []Habilidad{{ID: "Soundproof", Nombre: "Insonorizar"}, {ID: "Filter", Nombre: "Filtro"}, {ID: "Technician", Nombre: "Experto"}},
		Movimientos: []Movimiento{
			{ID: "Confusion", Nombre: "Confusión", Tipo: "psychic", Categoria: ptr("special"), Potencia: ptr(50)},
			{ID: "Barrier", Nombre: "Barrier", Tipo: "psychic", Categoria: ptr("status")},
			{ID: "raro", Nombre: "raro", Tipo: "normal"},
		},
	})
}

func TestRepoPokemonFormaYHabilidadesPasadas(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM pokemon p`).WithArgs("gengar").WillReturnRows(sqlmock.NewRows(columnasPokemon).
		AddRow(94, "gengar", 1, "gengar", "Gengar", nil, "ghost", "poison", nil, nil, 1))
	// Gengar tenía Levitate hasta gen 6 (pasada en gen 6) y Cursed Body desde gen 7.
	mock.ExpectQuery(`FROM pokemon_abilities pa`).WillReturnRows(sqlmock.NewRows(columnasHabilidades).
		AddRow(1, false, 130, 5, "cursed-body", "Cursed Body", "Cuerpo Maldito", 0).
		AddRow(1, false, 26, 3, "levitate", "Levitate", "Levitación", 6).
		AddRow(1, false, 26, 3, "levitate-vieja", "Levitate", "Levitación", 8).
		AddRow(2, false, 0, 0, "", "", "", 6))
	mock.ExpectQuery(`FROM pokemon_moves pm`).WillReturnRows(sqlmock.NewRows(columnasMovimientos))
	d, err := r.Pokemon(context.Background(), 4, "Gengar")
	if err != nil {
		t.Fatal(err)
	}
	igual(t, d.Habilidades, []Habilidad{{ID: "Levitate", Nombre: "Levitación"}})
	igual(t, d.Nombre, "Gengar")
	igual(t, d.Movimientos, []Movimiento{})
}

func TestRepoPokemonFormaRegional(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM pokemon p`).WithArgs("yamaskgalar").WillReturnRows(sqlmock.NewRows(columnasPokemon).
		AddRow(10179, "yamask-galar", 0, "yamask", "Yamask", "Yamask", "ground", "ghost", "Yamask de Galar", nil, 8))
	mock.ExpectQuery(`FROM pokemon_abilities pa`).WillReturnRows(sqlmock.NewRows(columnasHabilidades))
	mock.ExpectQuery(`FROM pokemon_moves pm`).WillReturnRows(sqlmock.NewRows(columnasMovimientos))
	d, err := r.Pokemon(context.Background(), 8, "Yamask-Galar")
	if err != nil {
		t.Fatal(err)
	}
	if d.Especie != "Yamask-Galar" || d.Nombre != "Yamask de Galar" || len(d.Habilidades) != 0 || d.Habilidades == nil {
		t.Fatalf("detalle %+v", d)
	}
}

func TestRepoPokemonGen1y2SinHabilidades(t *testing.T) {
	r, mock := repoFalsoSQL(t)
	mock.ExpectQuery(`FROM pokemon p`).WillReturnRows(sqlmock.NewRows(columnasPokemon).
		AddRow(25, "pikachu", 1, "pikachu", "Pikachu", "Pikachu", "electric", nil, nil, nil, 1))
	mock.ExpectQuery(`FROM pokemon_moves pm`).WithArgs(25, 2).WillReturnRows(sqlmock.NewRows(columnasMovimientos))
	d, err := r.Pokemon(context.Background(), 2, "pikachu")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Habilidades) != 0 || d.Habilidades == nil {
		t.Fatalf("habilidades %#v", d.Habilidades)
	}
}

func TestRepoPokemonNoEncontrado(t *testing.T) {
	t.Run("nombre vacío tras normalizar: no consulta", func(t *testing.T) {
		r, _ := repoFalsoSQL(t)
		if _, err := r.Pokemon(context.Background(), 4, " .-' "); !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("sin filas", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM pokemon p`).WillReturnRows(sqlmock.NewRows(columnasPokemon))
		if _, err := r.Pokemon(context.Background(), 4, "missingno"); !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("de una generación posterior", func(t *testing.T) {
		r, mock := repoFalsoSQL(t)
		mock.ExpectQuery(`FROM pokemon p`).WillReturnRows(sqlmock.NewRows(columnasPokemon).
			AddRow(700, "sylveon", 1, "sylveon", "Sylveon", "Sylveon", "fairy", nil, nil, nil, 6))
		if _, err := r.Pokemon(context.Background(), 4, "sylveon"); !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestRepoPokemonErrores(t *testing.T) {
	fila := func() *sqlmock.Rows {
		return sqlmock.NewRows(columnasPokemon).AddRow(1, "bulbasaur", 1, "bulbasaur", "Bulbasaur", nil, "grass", "poison", nil, nil, 1)
	}
	casos := []struct {
		nombre   string
		preparar func(sqlmock.Sqlmock)
	}{
		{"pokémon", func(m sqlmock.Sqlmock) { m.ExpectQuery(`FROM pokemon p`).WillReturnError(errBase) }},
		{"habilidades", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM pokemon p`).WillReturnRows(fila())
			m.ExpectQuery(`pokemon_abilities`).WillReturnError(errBase)
		}},
		{"scan de habilidad", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM pokemon p`).WillReturnRows(fila())
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidades).AddRow("x", false, 1, 3, "a", "A", "A", 0))
		}},
		{"recorrer habilidades", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM pokemon p`).WillReturnRows(fila())
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidades).AddRow(1, false, 1, 3, "a", "A", "A", 0).RowError(0, errBase))
		}},
		{"movimientos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM pokemon p`).WillReturnRows(fila())
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidades))
			m.ExpectQuery(`pokemon_moves`).WillReturnError(errBase)
		}},
		{"scan de movimiento", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM pokemon p`).WillReturnRows(fila())
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidades))
			m.ExpectQuery(`pokemon_moves`).WillReturnRows(sqlmock.NewRows(columnasMovimientos).AddRow("a", "A", "A", "normal", "physical", "x"))
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			r, mock := repoFalsoSQL(t)
			caso.preparar(mock)
			if _, err := r.Pokemon(context.Background(), 4, "bulbasaur"); err == nil || errors.Is(err, ErrNoEncontrado) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func TestAyudantesDelRepositorio(t *testing.T) {
	t.Run("separarNombres", func(t *testing.T) {
		igual(t, separarNombres(""), []string(nil))
		igual(t, separarNombres("  "), []string(nil))
		igual(t, separarNombres("Rojo| Azul ||Amarillo"), []string{"Rojo", "Azul", "Amarillo"})
	})
	t.Run("separarLista", func(t *testing.T) {
		igual(t, separarLista(""), []string{})
		igual(t, separarLista("Tackle, Growl,  , Ember "), []string{"Tackle", "Growl", "Ember"})
	})
	t.Run("tipos", func(t *testing.T) {
		igual(t, tipos("", ""), []string{})
		igual(t, tipos("fire", ""), []string{"fire"})
		igual(t, tipos("", "flying"), []string{"flying"})
		igual(t, tipos("fire", "flying"), []string{"fire", "flying"})
	})
	t.Run("primero", func(t *testing.T) {
		igual(t, primero(), "")
		igual(t, primero("", "", "c", "d"), "c")
		igual(t, primero("a"), "a")
	})
	t.Run("escaparLike", func(t *testing.T) {
		igual(t, escaparLike(`pika`), `pika`)
		igual(t, escaparLike(`100%_\`), `100\%\_\\`)
	})
	t.Run("normalizarEspecie", func(t *testing.T) {
		igual(t, normalizarEspecie("  Mr. Mime "), "mrmime")
		igual(t, normalizarEspecie("Flabébé"), "flabebe")
		igual(t, normalizarEspecie("Yamask-Galar"), "yamaskgalar")
		igual(t, normalizarEspecie("?!"), "")
	})
	t.Run("sortInts", func(t *testing.T) {
		v := []int{3, 1, 2, 1, 0}
		sortInts(v)
		igual(t, v, []int{0, 1, 1, 2, 3})
		var vacio []int
		sortInts(vacio)
	})
}
