package pokemons

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"PokeTeamCalculator/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

var columnasPokemon = []string{
	"id", "identifier", "species_id", "height", "weight", "base_experience", "sort_order", "is_default", "type_ids", "type_names",
}

// conBaseFalsa sustituye la base global por sqlmock durante el test.
func conBaseFalsa(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	anterior := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = anterior
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return mock
}

func esperarCantidad(mock sqlmock.Sqlmock, n int64) {
	mock.ExpectQuery(`name: GetPokemonQuantity`).WillReturnRows(sqlmock.NewRows([]string{"pokemons"}).AddRow(n))
}

func filas(desde, cuantas int) *sqlmock.Rows {
	r := sqlmock.NewRows(columnasPokemon)
	for i := desde; i < desde+cuantas; i++ {
		r.AddRow(i, "poke-"+strconv.Itoa(i), i, 7, 69, 64, i, 1, "12, 4", "Planta, Veneno")
	}
	return r
}

func TestServiceGetPokemonsPaginas(t *testing.T) {
	casos := []struct {
		nombre     string
		pagina     int
		total      int64
		offset     int
		devueltos  int
		consulta   bool
		totalPags  int
		ultima     bool
		esperadosN int
	}{
		{"primera de varias", 1, 45, 0, 20, true, 3, false, 20},
		{"intermedia", 2, 45, 20, 20, true, 3, false, 20},
		{"última incompleta", 3, 45, 40, 5, true, 3, true, 5},
		{"última justa", 2, 40, 20, 20, true, 2, true, 20},
		{"fuera de rango: no consulta", 4, 45, 0, 0, false, 3, true, 0},
		{"sin pokémon", 1, 0, 0, 0, false, 0, true, 0},
		{"más allá del máximo", maxPagina + 1, 45, 0, 0, false, 3, true, 0},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			mock := conBaseFalsa(t)
			esperarCantidad(mock, caso.total)
			if caso.consulta {
				mock.ExpectQuery(`name: GetPokemonsPager`).WithArgs(caso.offset).WillReturnRows(filas(caso.offset+1, caso.devueltos))
			}

			p, err := ServiceGetPokemons(caso.pagina)
			if err != nil {
				t.Fatal(err)
			}
			if p.Page != caso.pagina || p.PageSize != pageSize || p.Total != caso.total || p.TotalPages != caso.totalPags || p.Last != caso.ultima {
				t.Fatalf("página %+v", p)
			}
			if p.Pokemons == nil || len(p.Pokemons) != caso.esperadosN {
				t.Fatalf("%d pokémon (nil=%v), esperados %d", len(p.Pokemons), p.Pokemons == nil, caso.esperadosN)
			}
		})
	}
}

func TestServiceGetPokemonsSinFilasDevuelveArrayVacio(t *testing.T) {
	mock := conBaseFalsa(t)
	esperarCantidad(mock, 10)
	mock.ExpectQuery(`name: GetPokemonsPager`).WithArgs(0).WillReturnRows(sqlmock.NewRows(columnasPokemon))
	p, err := ServiceGetPokemons(1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Pokemons == nil || len(p.Pokemons) != 0 || p.Last {
		t.Fatalf("página %+v", p)
	}
	salida, _ := json.Marshal(p)
	if !json.Valid(salida) || string(salida) == "" {
		t.Fatal("JSON inválido")
	}
}

func TestServiceGetPokemonsErrores(t *testing.T) {
	t.Run("página inválida", func(t *testing.T) {
		conBaseFalsa(t)
		if _, err := ServiceGetPokemons(0); !errors.Is(err, ErrPaginaInvalida) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("falla el recuento", func(t *testing.T) {
		mock := conBaseFalsa(t)
		mock.ExpectQuery(`name: GetPokemonQuantity`).WillReturnError(errors.New("caída"))
		if _, err := ServiceGetPokemons(1); err == nil || err.Error() != "caída" {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("falla la página", func(t *testing.T) {
		mock := conBaseFalsa(t)
		esperarCantidad(mock, 45)
		mock.ExpectQuery(`name: GetPokemonsPager`).WillReturnError(errors.New("caída"))
		if _, err := ServiceGetPokemons(1); err == nil {
			t.Fatal("se esperaba un error")
		}
	})
}

func servir(t *testing.T, h gin.HandlerFunc, ruta string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, ruta, nil)
	c.Params = params
	h(c)
	return w
}

func TestGetAllPokemons(t *testing.T) {
	t.Run("página por defecto", func(t *testing.T) {
		mock := conBaseFalsa(t)
		esperarCantidad(mock, 2)
		mock.ExpectQuery(`name: GetPokemonsPager`).WithArgs(0).WillReturnRows(filas(1, 2))
		w := servir(t, GetAllPokemons, "/api/v1/pokemon", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("código %d: %s", w.Code, w.Body)
		}
		var p PokemonPage
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if p.Page != 1 || len(p.Pokemons) != 2 || !p.Last || p.Pokemons[0].Identifier != "poke-1" || *p.Pokemons[0].TypeNames != "Planta, Veneno" {
			t.Fatalf("página %+v", p)
		}
	})

	t.Run("error de la base → 500 sin detalles", func(t *testing.T) {
		mock := conBaseFalsa(t)
		mock.ExpectQuery(`name: GetPokemonQuantity`).WillReturnError(errors.New("password=secreta"))
		w := servir(t, GetAllPokemons, "/api/v1/pokemon?page=2", nil)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("código %d", w.Code)
		}
		var cuerpo map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &cuerpo)
		if cuerpo["error"] != "pokemon_query_failed" {
			t.Fatalf("cuerpo %v", cuerpo)
		}
		if strings.Contains(w.Body.String(), "secreta") {
			t.Fatal("la respuesta filtra el error interno")
		}
	})
}

func TestGetPokemonByID(t *testing.T) {
	t.Run("encontrado", func(t *testing.T) {
		mock := conBaseFalsa(t)
		mock.ExpectQuery(`name: GetPokemonByID`).WithArgs(25).WillReturnRows(filas(25, 1))
		w := servir(t, GetPokemonByID, "/api/v1/pokemon/25", gin.Params{{Key: "id", Value: "25"}})
		if w.Code != http.StatusOK {
			t.Fatalf("código %d: %s", w.Code, w.Body)
		}
		var p map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if p["id"] != float64(25) || p["identifier"] != "poke-25" {
			t.Fatalf("cuerpo %v", p)
		}
	})

	t.Run("no existe → 404", func(t *testing.T) {
		mock := conBaseFalsa(t)
		mock.ExpectQuery(`name: GetPokemonByID`).WithArgs(9999).WillReturnRows(sqlmock.NewRows(columnasPokemon))
		w := servir(t, GetPokemonByID, "/api/v1/pokemon/9999", gin.Params{{Key: "id", Value: "9999"}})
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "pokemon_not_found") {
			t.Fatalf("código %d: %s", w.Code, w.Body)
		}
	})

	t.Run("error de la base → 500", func(t *testing.T) {
		mock := conBaseFalsa(t)
		mock.ExpectQuery(`name: GetPokemonByID`).WillReturnError(errors.New("caída"))
		w := servir(t, GetPokemonByID, "/api/v1/pokemon/1", gin.Params{{Key: "id", Value: "1"}})
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "pokemon_query_failed") {
			t.Fatalf("código %d: %s", w.Code, w.Body)
		}
	})
}
