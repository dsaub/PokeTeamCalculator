package simulaciones

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCookieFirmada(t *testing.T) {
	s := NewSessions([]byte("0123456789abcdef0123456789abcdef"))
	sid := nuevoSID()
	if len(sid) != 32 {
		t.Fatalf("el SID debe tener 32 hex, tiene %d", len(sid))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: nombreCookieSID, Value: sid + "." + s.firma(sid)})
	got, ok := s.leer(req)
	if !ok || got != sid {
		t.Fatalf("cookie válida rechazada: %q %v", got, ok)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: nombreCookieSID, Value: sid + ".firmafalsa"})
	if _, ok := s.leer(req2); ok {
		t.Fatal("se aceptó una cookie con firma incorrecta")
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: nombreCookieSID, Value: sid})
	if _, ok := s.leer(req3); ok {
		t.Fatal("se aceptó una cookie sin firma")
	}
}

func TestEnsureEmiteCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewSessions([]byte("0123456789abcdef0123456789abcdef"))
	r := gin.New()
	r.Use(s.Ensure)
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, UsuarioActual(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("código %d", rec.Code)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, nombreCookieSID+"=") {
		t.Fatalf("no se emitió la cookie: %q", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("faltan atributos de seguridad: %q", cookie)
	}
	if !strings.Contains(cookie, "Secure") {
		t.Fatalf("la cookie debería ser Secure con X-Forwarded-Proto=https: %q", cookie)
	}
}

func TestEnsureReutilizaCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewSessions([]byte("0123456789abcdef0123456789abcdef"))
	r := gin.New()
	r.Use(s.Ensure)
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, UsuarioActual(c)) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	cookie, err := http.ParseSetCookie(rec.Header().Get("Set-Cookie"))
	if err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if rec2.Body.String() != rec.Body.String() {
		t.Fatalf("el usuario cambió: %q != %q", rec2.Body.String(), rec.Body.String())
	}
	if rec2.Header().Get("Set-Cookie") != "" {
		t.Fatal("no debería reemitirse la cookie si es válida")
	}
}
