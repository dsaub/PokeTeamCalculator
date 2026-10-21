package simulaciones

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type apiPrueba struct {
	cfg     Config
	repo    *memRepo
	waker   *wakerFalso
	catalog *catalogFalso
	svc     *Service
	hub     *Hub
	rt      *Realtime
	handler *Handler
	router  *gin.Engine
}

type catalogFalso struct {
	mu       sync.Mutex
	existe   bool
	valida   bool
	err      error
	llamadas int
}

func (c *catalogFalso) HabilidadValida(context.Context, int, string, string) (bool, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.llamadas++
	return c.existe, c.valida, c.err
}

func (c *catalogFalso) numLlamadas() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.llamadas
}

func (c *catalogFalso) configurar(existe, valida bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.existe, c.valida = existe, valida
}

func configPrueba() Config {
	return Config{
		TokenInterno:            "token-de-prueba",
		URLWorker:               "http://127.0.0.1:7070",
		PuertoInterno:           "8081",
		SecretoCookie:           []byte("0123456789abcdef0123456789abcdef"),
		Simulaciones:            300,
		AleatoriedadRival:       0.2,
		TrabajosSimultaneos:     2,
		MaxPendientesPorUsuario: 1,
		MaxCola:                 200,
		CacheTTL:                168 * time.Hour,
		Retencion:               720 * time.Hour,
		CancelarSinCliente:      2 * time.Minute,
		IntervaloSondeoSSE:      5 * time.Second,
		LimitePorMinuto:         1000,
	}
}

func nuevaAPI(t *testing.T, ajustar func(*Config, *memRepo)) *apiPrueba {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := configPrueba()
	repo := newMemRepo()
	if ajustar != nil {
		ajustar(&cfg, repo)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	waker := &wakerFalso{disponible: true}
	catalog := &catalogFalso{existe: true, valida: true}
	svc := NewService(repo, cfg, waker, logger, catalog)
	hub := NewHub()
	rt := NewRealtime(hub, svc, cfg.IntervaloSondeoSSE, logger)
	handler := NewHandler(svc, rt, cfg, logger)

	router := gin.New()
	handler.Register(router)

	return &apiPrueba{
		cfg:     cfg,
		repo:    repo,
		waker:   waker,
		catalog: catalog,
		svc:     svc,
		hub:     hub,
		rt:      rt,
		handler: handler,
		router:  router,
	}
}

func (a *apiPrueba) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(a.router)
	t.Cleanup(s.Close)
	return s
}

func nuevoCliente(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func hacerJSON(t *testing.T, cliente *http.Client, metodo, url string, cuerpo any) (int, []byte, http.Header) {
	t.Helper()
	var lector io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatal(err)
		}
		lector = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, url, lector)
	if err != nil {
		t.Fatal(err)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cliente.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, datos, resp.Header
}

func hacerSinCuerpo(t *testing.T, cliente *http.Client, metodo, url string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(metodo, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cliente.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, datos
}

func cuerpoValido() map[string]any {
	return map[string]any{
		"gen": "gen4",
		"rival": map[string]any{
			"especie":     "Cranidos",
			"nivel":       14,
			"habilidad":   "Mold Breaker",
			"movimientos": []string{"Headbutt", "Pursuit", "Leer"},
		},
		"candidatos": []any{
			map[string]any{
				"especie":     "Monferno",
				"nivel":       16,
				"naturaleza":  "Adamant",
				"evs":         map[string]any{"atk": 20},
				"movimientos": []string{"Mach Punch", "Ember"},
			},
			map[string]any{
				"especie":     "Grotle",
				"nivel":       18,
				"movimientos": []string{"Razor Leaf", "Bite"},
			},
		},
	}
}

func decodificar(t *testing.T, datos []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("JSON inválido: %v (%s)", err, datos)
	}
	return m
}

func peticionDeCuerpo(t *testing.T, cuerpo map[string]any) Peticion {
	t.Helper()
	b, err := json.Marshal(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	var p PeticionCliente
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return Peticion{Gen: p.Gen, Rival: p.Rival, Candidatos: p.Candidatos}
}

func conServidorYDefaults(t *testing.T, p Peticion, cfg Config) Peticion {
	t.Helper()
	p.Simulaciones = cfg.Simulaciones
	p.Opciones = &Opciones{AleatoriedadRival: &cfg.AleatoriedadRival}
	return p
}
