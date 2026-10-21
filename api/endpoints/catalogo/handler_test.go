package catalogo

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type repoFalso struct {
	juegos   []Juego
	lideres  []Lider
	resumen  []PokemonResumen
	detalle  *PokemonDetalle
	err      error
	busqueda string
	gen      int
}

func (r *repoFalso) Juegos(context.Context) ([]Juego, error) {
	return r.juegos, r.err
}
func (r *repoFalso) Lideres(context.Context, string, int) ([]Lider, error) {
	return r.lideres, r.err
}
func (r *repoFalso) BuscarPokemon(_ context.Context, gen int, q string, _ int) ([]PokemonResumen, error) {
	r.gen, r.busqueda = gen, q
	return r.resumen, r.err
}
func (r *repoFalso) Pokemon(context.Context, int, string) (*PokemonDetalle, error) {
	return r.detalle, r.err
}

func nuevoHandler(t *testing.T, repo Repo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(NewService(repo), logger)
	r := gin.New()
	h.Register(r)
	return r
}

func pedir(t *testing.T, r http.Handler, ruta string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var cuerpo map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &cuerpo)
	}
	return rec, cuerpo
}

func TestJuegosYCache(t *testing.T) {
	repo := &repoFalso{juegos: []Juego{{
		ID: "platino", Nombre: "Pokémon Platino", Gen: "gen4", VersionGroup: "platinum",
	}}}
	r := nuevoHandler(t, repo)
	rec, cuerpo := pedir(t, r, "/api/juegos")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("Cache-Control: %q", rec.Header().Get("Cache-Control"))
	}
	// La respuesta es un array; volvemos a decodificar como array.
	_ = cuerpo
	var juegos []Juego
	if err := json.Unmarshal(rec.Body.Bytes(), &juegos); err != nil {
		t.Fatal(err)
	}
	if len(juegos) != 1 || juegos[0].ID != "platino" || juegos[0].Nombre != "Pokémon Platino" {
		t.Fatalf("juegos: %+v", juegos)
	}
}

func TestLideres(t *testing.T) {
	habilidad := "Mold Breaker"
	repo := &repoFalso{
		juegos: []Juego{{ID: "platino", Nombre: "Pokémon Platino", Gen: "gen4", VersionGroup: "platinum"}},
		lideres: []Lider{{
			ID: 123, Nombre: "Roco", Ciudad: "Ciudad Pirita", Orden: 1,
			Equipo: []PokemonEquipo{{
				Especie: "Cranidos", Nombre: "Cranidos", Nivel: 14,
				Habilidad: &habilidad, Movimientos: []string{"Headbutt"}, Tipos: []string{"rock"},
			}},
		}},
	}
	r := nuevoHandler(t, repo)
	rec, _ := pedir(t, r, "/api/juegos/platino/lideres")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("Cache-Control: %q", rec.Header().Get("Cache-Control"))
	}
	var lideres []Lider
	if err := json.Unmarshal(rec.Body.Bytes(), &lideres); err != nil {
		t.Fatal(err)
	}
	if len(lideres) != 1 || lideres[0].Equipo[0].Especie != "Cranidos" {
		t.Fatalf("lideres: %+v", lideres)
	}
}

func TestLideresJuegoDesconocido(t *testing.T) {
	r := nuevoHandler(t, &repoFalso{juegos: []Juego{}})
	rec, _ := pedir(t, r, "/api/juegos/no-existe/lideres")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestBuscarValidacion(t *testing.T) {
	r := nuevoHandler(t, &repoFalso{})
	casos := []string{
		"/api/pokemon?gen=gen4&q=m",
		"/api/pokemon?gen=gen4",
		"/api/pokemon?q=monf",
		"/api/pokemon?gen=gen10&q=monf",
	}
	for _, ruta := range casos {
		rec, _ := pedir(t, r, ruta)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", ruta, rec.Code)
		}
	}
}

func TestBuscarPokemon(t *testing.T) {
	repo := &repoFalso{resumen: []PokemonResumen{{
		Especie: "Monferno", Nombre: "Monferno", Tipos: []string{"fire", "fighting"},
	}}}
	r := nuevoHandler(t, repo)
	rec, _ := pedir(t, r, "/api/pokemon?gen=gen4&q=monf")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if repo.gen != 4 || repo.busqueda != "monf" {
		t.Fatalf("consulta inesperada: gen=%d q=%q", repo.gen, repo.busqueda)
	}
	var resumen []PokemonResumen
	if err := json.Unmarshal(rec.Body.Bytes(), &resumen); err != nil {
		t.Fatal(err)
	}
	if len(resumen) != 1 || resumen[0].Especie != "Monferno" {
		t.Fatalf("resumen: %+v", resumen)
	}
}

func TestDetallePokemon(t *testing.T) {
	repo := &repoFalso{detalle: &PokemonDetalle{
		Especie: "Monferno", Nombre: "Monferno", Tipos: []string{"fire", "fighting"},
		Habilidades: []Habilidad{{ID: "Blaze", Nombre: "Mar Llamas"}},
		Movimientos: []Movimiento{{ID: "Mach Punch", Nombre: "Puño Máquina", Tipo: "fighting"}},
	}}
	r := nuevoHandler(t, repo)
	rec, _ := pedir(t, r, "/api/pokemon/Monferno?gen=gen4")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var detalle PokemonDetalle
	if err := json.Unmarshal(rec.Body.Bytes(), &detalle); err != nil {
		t.Fatal(err)
	}
	if detalle.Especie != "Monferno" || len(detalle.Habilidades) != 1 || len(detalle.Movimientos) != 1 {
		t.Fatalf("detalle: %+v", detalle)
	}
}

func TestDetalleNoEncontrado(t *testing.T) {
	r := nuevoHandler(t, &repoFalso{err: ErrNoEncontrado})
	rec, _ := pedir(t, r, "/api/pokemon/NoExiste?gen=gen4")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestGenInvalidaDetalle(t *testing.T) {
	r := nuevoHandler(t, &repoFalso{})
	rec, _ := pedir(t, r, "/api/pokemon/Monferno?gen=gen0")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}
