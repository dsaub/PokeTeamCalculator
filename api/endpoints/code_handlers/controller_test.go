package codehandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespuestasDeError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nombre  string
		handler gin.HandlerFunc
		codigo  int
		error   string
		mensaje string
	}{
		{"404", ErrorNotFound, http.StatusNotFound, "endpoint_not_found", "The requested resource does not exist"},
		{"405", ErrorMethodNotAllowed, http.StatusMethodNotAllowed, "method_not_allowed", "The requested method is not allowed for this resource"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)

			caso.handler(c)

			if w.Code != caso.codigo {
				t.Fatalf("código %d, esperado %d", w.Code, caso.codigo)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type %q", ct)
			}
			var cuerpo map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("respuesta no JSON: %v", err)
			}
			if cuerpo["status"] != float64(caso.codigo) || cuerpo["error"] != caso.error || cuerpo["message"] != caso.mensaje {
				t.Fatalf("cuerpo %v", cuerpo)
			}
		})
	}
}
