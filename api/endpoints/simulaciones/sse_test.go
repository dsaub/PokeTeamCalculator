package simulaciones

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestSSEInicialYCompletado(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
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

	a.tomar(t, "w1")
	a.aplicarEstados(t)
	if evento, datos = leerEvento(t, sse); evento != "procesando" {
		t.Fatalf("se esperaba procesando: %q %s", evento, datos)
	}

	resultado := `{"gen":"gen4","rival":"Cranidos (Nv. 14)","simulaciones":300,"resultados":[{"pokemon":"Monferno","nivel":16,"victorias":300,"derrotas":0,"empates":0,"winrate":1,"psMedioVictoria":0.5,"turnosMedio":4}],"ms":10}`
	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1", "resultado", resultado)
	a.aplicarEstados(t)

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
	if job, _ := a.repo.Get(context.Background(), id); job.Estado != EstadoCompletado || string(job.Resultado) != resultado {
		t.Fatalf("MariaDB no recibió el resultado: %+v", job)
	}
	if n, _ := a.cola.EstadosSinAplicar(context.Background()); n != 0 {
		t.Fatalf("quedan %d estados sin confirmar", n)
	}
}

func TestSSEVariosClientes(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d := crearHTTP(t, srv, cliente, cuerpoValido())
	id := int64(d["id"].(float64))

	sse1 := abrirSSE(t, srv, cliente, id)
	sse2 := abrirSSE(t, srv, cliente, id)
	leerEvento(t, sse1)
	leerEvento(t, sse2)

	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1", "resultado", `{"gen":"gen4","resultados":[]}`)
	a.aplicarEstados(t)

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

func TestSSEPosicionesAlTomarUnTrabajo(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
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

	if tomado := a.tomar(t, "w1"); tomado != id1 {
		t.Fatalf("el worker tomó %d antes que %d", tomado, id1)
	}
	a.aplicarEstados(t)

	evento, datos = leerEvento(t, sse)
	if evento != "posicion" {
		t.Fatalf("se esperaba nueva posición: %s %s", evento, datos)
	}
	if !strings.Contains(datos, `"posicion":1`) {
		t.Fatalf("posición tras avanzar la cola: %s", datos)
	}
}

// El requisito de la cola: un trabajo que falla vuelve detrás de los que ya
// esperaban, y el jugador lo ve en su posición.
func TestSSEReintentoVuelveAlFinal(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d1 := crearHTTP(t, srv, c1, cuerpoValido())
	id1 := int64(d1["id"].(float64))
	cuerpo2 := cuerpoValido()
	cuerpo2["rival"].(map[string]any)["especie"] = "Shieldon"
	_, d2 := crearHTTP(t, srv, c2, cuerpo2)
	id2 := int64(d2["id"].(float64))

	sse := abrirSSE(t, srv, c1, id1)
	if evento, datos := leerEvento(t, sse); evento != "posicion" || !strings.Contains(datos, `"posicion":1`) {
		t.Fatalf("posición inicial: %s %s", evento, datos)
	}

	a.tomar(t, "w1")
	a.aplicarEstados(t)
	if evento, _ := leerEvento(t, sse); evento != "procesando" {
		t.Fatalf("se esperaba procesando: %q", evento)
	}

	a.reintentar(t, "w1", id1)
	a.aplicarEstados(t)
	if cola := a.enCola(t); len(cola) != 2 || cola[0] != id2 || cola[1] != id1 {
		t.Fatalf("cola tras el fallo %v, se esperaba [%d %d]", cola, id2, id1)
	}
	evento, datos := leerEvento(t, sse)
	if evento != "posicion" || !strings.Contains(datos, `"posicion":2`) {
		t.Fatalf("tras el fallo debía quedar el segundo: %s %s", evento, datos)
	}
	if a.repo.estado(id1) != EstadoPendiente {
		t.Fatalf("estado en MariaDB %q", a.repo.estado(id1))
	}
	_, cuerpo := hacerSinCuerpo(t, c1, http.MethodGet, fmt.Sprintf("%s/api/simulaciones/%d", srv.URL, id1))
	if m := decodificar(t, cuerpo); m["estado"] != EstadoPendiente || m["posicion"].(float64) != 2 {
		t.Fatalf("GET tras el fallo: %s", cuerpo)
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
