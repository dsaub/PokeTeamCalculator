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
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type apiPrueba struct {
	cfg     Config
	repo    *memRepo
	mr      *miniredis.Miniredis
	rdb     *redis.Client
	cola    *ColaRedis
	estados *Estados
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
		URLRedis:                "redis://127.0.0.1:6379",
		LatidoCaducadoWorker:    15 * time.Second,
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
	mr := miniredis.RunT(t)
	// Sin reintentos: los tests que cierran miniredis fallan al momento.
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialerRetries: 1})
	t.Cleanup(func() { _ = rdb.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cola := NewColaRedis(rdb, cfg.LatidoCaducadoWorker, logger)
	catalog := &catalogFalso{existe: true, valida: true}
	svc := NewService(repo, cfg, cola, logger, catalog)
	hub := NewHub()
	rt := NewRealtime(hub, svc, cola, cfg.IntervaloSondeoSSE, logger)
	handler := NewHandler(svc, rt, cfg, logger)

	router := gin.New()
	handler.Register(router)

	a := &apiPrueba{
		cfg:     cfg,
		repo:    repo,
		mr:      mr,
		rdb:     rdb,
		cola:    cola,
		estados: NewEstados(cola, svc, rt, logger),
		catalog: catalog,
		svc:     svc,
		hub:     hub,
		rt:      rt,
		handler: handler,
		router:  router,
	}
	// Por defecto hay un worker vivo.
	a.latir(t, "w1")
	return a
}

// latir registra el latido de un worker con la hora de Redis, como el worker.
func (a *apiPrueba) latir(t *testing.T, worker string) {
	t.Helper()
	if _, err := a.mr.ZAdd(claveWorkers, float64(time.Now().UnixMilli()), worker); err != nil {
		t.Fatal(err)
	}
	a.cola.Comprobar(context.Background())
}

// sinWorkers borra todos los latidos.
func (a *apiPrueba) sinWorkers(t *testing.T) {
	t.Helper()
	a.mr.Del(claveWorkers)
	a.cola.Comprobar(context.Background())
}

// enCola devuelve la cola de Redis en orden de salida: el primero es el
// siguiente que tomará un worker.
func (a *apiPrueba) enCola(t *testing.T) []int64 {
	t.Helper()
	ids, err := a.rdb.LRange(context.Background(), claveCola, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	cola := make([]int64, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		id, err := strconv.ParseInt(ids[i], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		cola = append(cola, id)
	}
	return cola
}

// tomar hace lo que el worker al tomar un trabajo: lo saca de la cola, gasta
// un intento y publica `procesando`. Devuelve el id.
func (a *apiPrueba) tomar(t *testing.T, worker string) int64 {
	t.Helper()
	ctx := context.Background()
	s, err := a.rdb.LMove(ctx, claveCola, "sim:procesando:"+worker, "RIGHT", "LEFT").Result()
	if err != nil {
		t.Fatalf("no había nada en la cola: %v", err)
	}
	id, _ := strconv.ParseInt(s, 10, 64)
	intentos, err := a.rdb.HIncrBy(ctx, claveTrabajo(id), "intentos", 1).Result()
	if err != nil {
		t.Fatal(err)
	}
	a.publicarEstado(t, "id", id, "tipo", EstadoProcesando, "intentos", intentos, "worker", worker)
	return id
}

// reintentar hace lo que el worker con un fallo reintentable: el trabajo
// vuelve al final de la cola y publica `pendiente`.
func (a *apiPrueba) reintentar(t *testing.T, worker string, id int64) {
	t.Helper()
	ctx := context.Background()
	idTexto := strconv.FormatInt(id, 10)
	if err := a.rdb.LRem(ctx, "sim:procesando:"+worker, 0, idTexto).Err(); err != nil {
		t.Fatal(err)
	}
	if err := a.rdb.LPush(ctx, claveCola, idTexto).Err(); err != nil {
		t.Fatal(err)
	}
	intentos, _ := a.rdb.HGet(ctx, claveTrabajo(id), "intentos").Int()
	a.publicarEstado(t, "id", id, "tipo", EstadoPendiente, "motivo", "reintento", "intentos", intentos, "worker", worker)
}

// publicarEstado escribe una entrada en sim:estados, como el worker.
func (a *apiPrueba) publicarEstado(t *testing.T, campos ...any) {
	t.Helper()
	if err := a.rdb.XAdd(context.Background(), &redis.XAddArgs{Stream: claveEstados, Values: campos}).Err(); err != nil {
		t.Fatal(err)
	}
}

// aplicarEstados hace que la API aplique lo que haya en sim:estados.
func (a *apiPrueba) aplicarEstados(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := a.cola.prepararGrupo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.estados.Procesar(ctx, -1); err != nil {
		t.Fatal(err)
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
