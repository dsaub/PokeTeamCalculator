package simulaciones

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type conexionSSE struct {
	resp   *http.Response
	lector *bufio.Reader
	cancel context.CancelFunc
}

func abrirSSE(t *testing.T, srv *httptest.Server, cliente *http.Client, id int64) *conexionSSE {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/simulaciones/%d/eventos", srv.URL, id), nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp, err := cliente.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		cancel()
		t.Fatalf("SSE status %d: %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("Content-Type %q", ct)
	}
	c := &conexionSSE{resp: resp, lector: bufio.NewReader(resp.Body), cancel: cancel}
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	return c
}

func leerEvento(t *testing.T, c *conexionSSE) (string, string) {
	t.Helper()
	type resultado struct {
		evento, datos string
		err           error
	}
	ch := make(chan resultado, 1)
	go func() {
		evento, datos, err := parsearEvento(c.lector)
		ch <- resultado{evento, datos, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("leyendo SSE: %v", r.err)
		}
		return r.evento, r.datos
	case <-time.After(5 * time.Second):
		c.cancel()
		t.Fatal("timeout esperando un evento SSE")
		return "", ""
	}
}

func parsearEvento(r *bufio.Reader) (string, string, error) {
	var evento, datos string
	for {
		linea, err := r.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		linea = strings.TrimRight(linea, "\r\n")
		if linea == "" {
			return evento, datos, nil
		}
		if strings.HasPrefix(linea, "event: ") {
			evento = strings.TrimPrefix(linea, "event: ")
		}
		if strings.HasPrefix(linea, "data: ") {
			datos = strings.TrimPrefix(linea, "data: ")
		}
	}
}

func esperarCierre(t *testing.T, c *conexionSSE) {
	t.Helper()
	ch := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(c.lector)
		ch <- err
	}()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatalf("se esperaba EOF: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("la conexión SSE no se cerró")
	}
}

func llamarCallback(t *testing.T, urlInterna, token string, id int64) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/interno/simulaciones/%d/lista", urlInterna, id), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("callback status %d: %s", resp.StatusCode, b)
	}
}

func servidorInterno(t *testing.T, a *apiPrueba) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := httptest.NewServer(NewInternalHandler(a.svc, a.rt, a.cfg.TokenInterno, logger))
	t.Cleanup(s.Close)
	return s
}

func TestSSEInicialYCompletado(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	interno := servidorInterno(t, a)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))

	sse := abrirSSE(t, srv, cliente, id)
	evento, datos := leerEvento(t, sse)
	if evento != "posicion" {
		t.Fatalf("evento inicial %q", evento)
	}
	var pos struct {
		Posicion int `json:"posicion"`
	}
	if err := json.Unmarshal([]byte(datos), &pos); err != nil || pos.Posicion != 1 {
		t.Fatalf("posición inicial: %s (%v)", datos, err)
	}

	resultado := `{"gen":"gen4","rival":"Cranidos (Nv. 14)","simulaciones":300,"resultados":[{"pokemon":"Monferno","nivel":16,"victorias":300,"derrotas":0,"empates":0,"winrate":1,"psMedioVictoria":0.5,"turnosMedio":4}],"ms":10}`
	a.repo.setEstado(id, EstadoCompletado, []byte(resultado), "")
	llamarCallback(t, interno.URL, a.cfg.TokenInterno, id)

	evento, datos = leerEvento(t, sse)
	if evento != "completado" {
		t.Fatalf("evento final %q: %s", evento, datos)
	}
	var fin struct {
		Resultado map[string]any `json:"resultado"`
	}
	if err := json.Unmarshal([]byte(datos), &fin); err != nil {
		t.Fatal(err)
	}
	if fin.Resultado["gen"] != "gen4" {
		t.Fatalf("resultado SSE: %s", datos)
	}
	esperarCierre(t, sse)
}

func TestSSEVariosClientes(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	interno := servidorInterno(t, a)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))

	sse1 := abrirSSE(t, srv, cliente, id)
	sse2 := abrirSSE(t, srv, cliente, id)
	leerEvento(t, sse1)
	leerEvento(t, sse2)

	a.repo.setEstado(id, EstadoCompletado, []byte(`{"gen":"gen4","resultados":[]}`), "")
	llamarCallback(t, interno.URL, a.cfg.TokenInterno, id)

	for i, sse := range []*conexionSSE{sse1, sse2} {
		evento, _ := leerEvento(t, sse)
		if evento != "completado" {
			t.Fatalf("cliente %d recibió %q", i+1, evento)
		}
	}
}

func TestSSEClienteDesconectado(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))

	sse := abrirSSE(t, srv, cliente, id)
	leerEvento(t, sse)
	sse.cancel()
	_ = sse.resp.Body.Close()

	for i := 0; i < 100; i++ {
		if !a.hub.HasSubscribers(id) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("el hub no liberó al cliente desconectado")
}

func TestSSESondeoRespaldo(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))
	sse := abrirSSE(t, srv, cliente, id)
	leerEvento(t, sse)

	a.repo.setProcesando(id)
	a.rt.PollOnce(context.Background())
	evento, _ := leerEvento(t, sse)
	if evento != "procesando" {
		t.Fatalf("se esperaba procesando: %q", evento)
	}

	a.repo.setEstado(id, EstadoCompletado, []byte(`{"gen":"gen4","resultados":[]}`), "")
	a.rt.PollOnce(context.Background())
	evento, _ = leerEvento(t, sse)
	if evento != "completado" {
		t.Fatalf("se esperaba completado: %q", evento)
	}
}

func TestSSEPosicionesTrasCallback(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	interno := servidorInterno(t, a)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d1 := crearHTTP(t, srv, c1, cuerpoValido())
	id1 := int64(d1["id"].(float64))

	cuerpo2 := cuerpoValido()
	cuerpo2["rival"].(map[string]any)["especie"] = "Shieldon"
	_, d2 := crearHTTP(t, srv, c2, cuerpo2)
	id2 := int64(d2["id"].(float64))

	sse := abrirSSE(t, srv, c2, id2)
	evento, datos := leerEvento(t, sse)
	if evento != "posicion" || !strings.Contains(datos, `"posicion":2`) {
		t.Fatalf("posición inicial de job2: %s %s", evento, datos)
	}

	a.repo.setEstado(id1, EstadoCompletado, []byte(`{"gen":"gen4","resultados":[]}`), "")
	llamarCallback(t, interno.URL, a.cfg.TokenInterno, id1)

	evento, datos = leerEvento(t, sse)
	if evento != "posicion" {
		t.Fatalf("se esperaba nueva posición: %s %s", evento, datos)
	}
	if !strings.Contains(datos, `"posicion":1`) {
		t.Fatalf("posición tras avanzar la cola: %s", datos)
	}
}

func TestSSECancelado(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))
	sse := abrirSSE(t, srv, cliente, id)
	leerEvento(t, sse)

	status, _ := hacerSinCuerpo(t, cliente, http.MethodDelete, fmt.Sprintf("%s/api/simulaciones/%d", srv.URL, id))
	if status != http.StatusNoContent {
		t.Fatalf("cancelar: %d", status)
	}
	evento, _ := leerEvento(t, sse)
	if evento != "cancelado" {
		t.Fatalf("se esperaba cancelado: %q", evento)
	}
	esperarCierre(t, sse)
}

func TestSSECompletadoInicial(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))
	a.repo.setEstado(id, EstadoCompletado, []byte(`{"gen":"gen4","resultados":[]}`), "")

	sse := abrirSSE(t, srv, cliente, id)
	evento, _ := leerEvento(t, sse)
	if evento != "completado" {
		t.Fatalf("se esperaba completado: %q", evento)
	}
	esperarCierre(t, sse)
}

func TestSSEAjeno404(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d := crearHTTP(t, srv, c1, cuerpoValido())
	id := int64(d["id"].(float64))

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/simulaciones/%d/eventos", srv.URL, id), nil)
	resp, err := c2.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("otro usuario: %d", resp.StatusCode)
	}
}
