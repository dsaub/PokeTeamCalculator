package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type moduloFalso struct{}

func (moduloFalso) Register(r *gin.Engine) {
	r.GET("/api/prueba", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
}

func pedir(t *testing.T, router http.Handler, metodo, ruta string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(metodo, ruta, nil))
	return w
}

func TestNuevoRouterRegistraLasRutas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := nuevoRouter(moduloFalso{})

	rutas := map[string]bool{}
	for _, r := range router.Routes() {
		rutas[r.Method+" "+r.Path] = true
	}
	for _, esperada := range []string{"GET /api/v1/pokemon", "GET /api/v1/pokemon/:id", "GET /api/prueba"} {
		if !rutas[esperada] {
			t.Fatalf("falta la ruta %s; hay %v", esperada, rutas)
		}
	}
	if w := pedir(t, router, http.MethodGet, "/api/prueba"); w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("módulo: %d %q", w.Code, w.Body.String())
	}
}

func TestNuevoRouterResponde404Y405EnJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := nuevoRouter(moduloFalso{})

	casos := []struct {
		nombre, metodo, ruta string
		codigo               int
		error                string
	}{
		{"ruta desconocida", http.MethodGet, "/no/existe", http.StatusNotFound, "endpoint_not_found"},
		{"método no permitido", http.MethodPost, "/api/prueba", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			w := pedir(t, router, caso.metodo, caso.ruta)
			if w.Code != caso.codigo {
				t.Fatalf("código %d, esperado %d", w.Code, caso.codigo)
			}
			var cuerpo map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("respuesta no JSON: %v", err)
			}
			if cuerpo["error"] != caso.error || cuerpo["status"] != float64(caso.codigo) {
				t.Fatalf("cuerpo %v", cuerpo)
			}
		})
	}
}

func TestPuerto(t *testing.T) {
	t.Setenv("PORT", "")
	if p := puerto(); p != "8080" {
		t.Fatalf("por defecto %q, esperado 8080", p)
	}
	t.Setenv("PORT", "9090")
	if p := puerto(); p != "9090" {
		t.Fatalf("con PORT %q, esperado 9090", p)
	}
}
