package pokemons

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetAllPokemonsRechazaPaginasInvalidas(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, ruta := range []string{"/api/v1/pokemon?page=0", "/api/v1/pokemon?page=-1", "/api/v1/pokemon?page=abc"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, ruta, nil)

		GetAllPokemons(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: código %d, esperado 400", ruta, w.Code)
		}
		var cuerpo map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("%s: respuesta no JSON: %v", ruta, err)
		}
		if cuerpo["error"] != "bad_page_entry" {
			t.Fatalf("%s: error %v", ruta, cuerpo["error"])
		}
	}
}

func TestGetPokemonByIDRechazaIDsInvalidos(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, id := range []string{"pikachu", "0", "-1"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/pokemon/"+id, nil)
		c.Params = gin.Params{{Key: "id", Value: id}}

		GetPokemonByID(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("id %q: código %d, esperado 400", id, w.Code)
		}
		if !strings.Contains(w.Body.String(), "bad_pokemon_id") {
			t.Fatalf("id %q: cuerpo %s", id, w.Body.String())
		}
	}
}
