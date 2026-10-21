package simulaciones

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func crearHTTP(t *testing.T, srv *httptest.Server, cliente *http.Client, cuerpo map[string]any) (int, map[string]any) {
	t.Helper()
	status, datos, _ := hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpo)
	return status, decodificar(t, datos)
}

func TestCrearSimulacion(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	status, datos, headers := hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido())
	if status != http.StatusAccepted {
		t.Fatalf("status %d: %s", status, datos)
	}
	m := decodificar(t, datos)
	id := int64(m["id"].(float64))
	if m["estado"] != EstadoPendiente {
		t.Fatalf("estado %v", m["estado"])
	}
	if m["posicion"].(float64) != 1 {
		t.Fatalf("posición %v", m["posicion"])
	}
	if m["estimadoSegundos"].(float64) != 5 {
		t.Fatalf("estimado %v", m["estimadoSegundos"])
	}
	if headers.Get("Set-Cookie") == "" {
		t.Fatal("falta la cookie de sesión")
	}
	if a.waker.numLlamadas() != 1 {
		t.Fatalf("waker llamado %d veces", a.waker.numLlamadas())
	}

	job, err := a.repo.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var p Peticion
	if err := json.Unmarshal(job.Peticion, &p); err != nil {
		t.Fatal(err)
	}
	if p.Simulaciones != 300 {
		t.Fatalf("simulaciones guardadas %d", p.Simulaciones)
	}
	if p.Opciones == nil || p.Opciones.AleatoriedadRival == nil || *p.Opciones.AleatoriedadRival != 0.2 {
		t.Fatalf("opciones guardadas %+v", p.Opciones)
	}
}

func TestCrearDesdeCache(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	p := conServidorYDefaults(t, peticionDeCuerpo(t, cuerpoValido()), a.cfg)
	hash, err := HashPeticion(p)
	if err != nil {
		t.Fatal(err)
	}
	cacheado := a.repo.addJob(Job{
		Hash:      hash,
		Usuario:   "otro-usuario",
		Peticion:  []byte(`{}`),
		Estado:    EstadoCompletado,
		Resultado: []byte(`{"gen":"gen4","rival":"Cranidos (Nv. 14)","simulaciones":300,"resultados":[],"ms":12}`),
		Terminado: sql.NullTime{Time: time.Now(), Valid: true},
	})

	status, datos, _ := hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido())
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, datos)
	}
	m := decodificar(t, datos)
	if m["cache"] != true {
		t.Fatalf("se esperaba cache: %s", datos)
	}
	if int64(m["id"].(float64)) != cacheado.ID {
		t.Fatalf("id cacheado %v != %d", m["id"], cacheado.ID)
	}
	if m["resultado"].(map[string]any)["gen"] != "gen4" {
		t.Fatalf("resultado inesperado: %s", datos)
	}
	if a.waker.numLlamadas() != 0 {
		t.Fatal("un acierto de caché no debe despertar al worker")
	}
}

func TestDuplicadoMismoUsuario(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, d1 := crearHTTP(t, srv, cliente, cuerpoValido())
	_, d2 := crearHTTP(t, srv, cliente, cuerpoValido())

	if int64(d1["id"].(float64)) != int64(d2["id"].(float64)) {
		t.Fatalf("ids distintos: %v %v", d1["id"], d2["id"])
	}
	if a.repo.count() != 1 {
		t.Fatalf("se crearon %d trabajos", a.repo.count())
	}
	if a.waker.numLlamadas() != 1 {
		t.Fatalf("waker llamado %d veces", a.waker.numLlamadas())
	}
}

func TestNoSeComparteEntreUsuarios(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d1 := crearHTTP(t, srv, c1, cuerpoValido())
	_, d2 := crearHTTP(t, srv, c2, cuerpoValido())

	if int64(d1["id"].(float64)) == int64(d2["id"].(float64)) {
		t.Fatal("dos usuarios no deben compartir un trabajo en curso")
	}
	if a.repo.count() != 2 {
		t.Fatalf("se crearon %d trabajos", a.repo.count())
	}
}

func TestLimitePorUsuario(t *testing.T) {
	a := nuevaAPI(t, func(cfg *Config, _ *memRepo) { cfg.MaxPendientesPorUsuario = 1 })
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	_, primero := crearHTTP(t, srv, cliente, cuerpoValido())
	idPrimero := int64(primero["id"].(float64))

	cuerpo := cuerpoValido()
	cuerpo["rival"].(map[string]any)["especie"] = "Shieldon"
	status, datos := crearHTTP(t, srv, cliente, cuerpo)
	if status != http.StatusTooManyRequests {
		t.Fatalf("status %d: %v", status, datos)
	}
	if int64(datos["id"].(float64)) != idPrimero {
		t.Fatalf("el 429 debería incluir el id %d: %v", idPrimero, datos)
	}
	if a.repo.count() != 1 {
		t.Fatalf("se crearon %d trabajos", a.repo.count())
	}
}

func TestLimiteGlobal(t *testing.T) {
	a := nuevaAPI(t, func(cfg *Config, repo *memRepo) {
		cfg.MaxCola = 1
		repo.addJob(Job{Hash: "otro", Usuario: "otro", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	})
	srv := a.servidor(t)

	status, datos := crearHTTP(t, srv, nuevoCliente(t), cuerpoValido())
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status %d: %v", status, datos)
	}
}

func TestLimitePorUsuarioConcurrente(t *testing.T) {
	a := nuevaAPI(t, func(cfg *Config, _ *memRepo) { cfg.MaxPendientesPorUsuario = 1 })
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	// Fijar la cookie antes de lanzar las dos peticiones.
	if status, _ := hacerSinCuerpo(t, cliente, http.MethodGet, srv.URL+"/api/salud"); status == 0 {
		t.Fatal("no se pudo inicializar la sesión")
	}

	cuerpos := []map[string]any{cuerpoValido(), cuerpoValido()}
	cuerpos[1]["rival"].(map[string]any)["especie"] = "Shieldon"

	type resultado struct {
		status int
		err    error
	}
	ch := make(chan resultado, 2)
	var wg sync.WaitGroup
	for _, cuerpo := range cuerpos {
		wg.Add(1)
		go func(cuerpo map[string]any) {
			defer wg.Done()
			b, _ := json.Marshal(cuerpo)
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/simulaciones", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			resp, err := cliente.Do(req)
			if err != nil {
				ch <- resultado{err: err}
				return
			}
			_ = resp.Body.Close()
			ch <- resultado{status: resp.StatusCode}
		}(cuerpo)
	}
	wg.Wait()
	close(ch)

	aceptadas, limitadas := 0, 0
	for r := range ch {
		if r.err != nil {
			t.Fatal(r.err)
		}
		switch r.status {
		case http.StatusAccepted:
			aceptadas++
		case http.StatusTooManyRequests:
			limitadas++
		}
	}
	if aceptadas != 1 || limitadas != 1 {
		t.Fatalf("aceptadas=%d limitadas=%d", aceptadas, limitadas)
	}
	if a.repo.count() != 1 {
		t.Fatalf("se crearon %d trabajos", a.repo.count())
	}
}

func TestWorkerNoDisponible(t *testing.T) {
	a := nuevaAPI(t, nil)
	a.waker.disponible = false
	srv := a.servidor(t)

	status, datos, _ := hacerJSON(t, nuevoCliente(t), http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido())
	if status != http.StatusAccepted {
		t.Fatalf("status %d", status)
	}
	m := decodificar(t, datos)
	if m["workerDisponible"] != false {
		t.Fatalf("se esperaba workerDisponible=false: %s", datos)
	}
}

func TestAccesoASimulacion(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d := crearHTTP(t, srv, c1, cuerpoValido())
	id := int64(d["id"].(float64))
	ruta := fmt.Sprintf("%s/api/simulaciones/%d", srv.URL, id)

	status, datos := hacerSinCuerpo(t, c1, http.MethodGet, ruta)
	if status != http.StatusOK {
		t.Fatalf("dueño: status %d %s", status, datos)
	}
	if decodificar(t, datos)["posicion"].(float64) != 1 {
		t.Fatalf("posición inesperada: %s", datos)
	}

	status, _ = hacerSinCuerpo(t, c2, http.MethodGet, ruta)
	if status != http.StatusNotFound {
		t.Fatalf("otro usuario: status %d", status)
	}

	a.repo.setEstado(id, EstadoCompletado, []byte(`{"gen":"gen4","resultados":[]}`), "")
	status, datos = hacerSinCuerpo(t, c2, http.MethodGet, ruta)
	if status != http.StatusOK {
		t.Fatalf("completado ajeno: status %d %s", status, datos)
	}
	if decodificar(t, datos)["estado"] != EstadoCompletado {
		t.Fatalf("estado inesperado: %s", datos)
	}

	if status, _ := hacerSinCuerpo(t, c1, http.MethodGet, srv.URL+"/api/simulaciones/abc"); status != http.StatusNotFound {
		t.Fatalf("id no numérico: status %d", status)
	}
	if status, _ := hacerSinCuerpo(t, c1, http.MethodGet, srv.URL+"/api/simulaciones/999999"); status != http.StatusNotFound {
		t.Fatalf("id inexistente: status %d", status)
	}
}

func TestCancelar(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	c1 := nuevoCliente(t)
	c2 := nuevoCliente(t)

	_, d := crearHTTP(t, srv, c1, cuerpoValido())
	id := int64(d["id"].(float64))
	ruta := fmt.Sprintf("%s/api/simulaciones/%d", srv.URL, id)

	if status, datos := hacerSinCuerpo(t, c2, http.MethodDelete, ruta); status != http.StatusNotFound {
		t.Fatalf("otro usuario canceló: %d %s", status, datos)
	}
	if status, datos := hacerSinCuerpo(t, c1, http.MethodDelete, ruta); status != http.StatusNoContent {
		t.Fatalf("cancelar pendiente: %d %s", status, datos)
	}
	_, datos := hacerSinCuerpo(t, c1, http.MethodGet, ruta)
	if decodificar(t, datos)["estado"] != EstadoCancelado {
		t.Fatalf("estado tras cancelar: %s", datos)
	}
	if status, _ := hacerSinCuerpo(t, c1, http.MethodDelete, ruta); status != http.StatusConflict {
		t.Fatalf("cancelar dos veces: %d", status)
	}

	// En proceso también se puede cancelar.
	cuerpo := cuerpoValido()
	cuerpo["rival"].(map[string]any)["especie"] = "Shieldon"
	_, d2 := crearHTTP(t, srv, c1, cuerpo)
	id2 := int64(d2["id"].(float64))
	a.repo.setProcesando(id2)
	ruta2 := fmt.Sprintf("%s/api/simulaciones/%d", srv.URL, id2)
	if status, datos := hacerSinCuerpo(t, c1, http.MethodDelete, ruta2); status != http.StatusNoContent {
		t.Fatalf("cancelar procesando: %d %s", status, datos)
	}
}

func TestSanearGeneracionesAntiguas(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)

	comprobar := func(gen string, esperaObjeto bool) {
		cuerpo := cuerpoValido()
		cuerpo["gen"] = gen
		afectados := []map[string]any{cuerpo["rival"].(map[string]any)}
		for _, c := range cuerpo["candidatos"].([]any) {
			afectados = append(afectados, c.(map[string]any))
		}
		for _, quien := range afectados {
			quien["habilidad"] = "Mold Breaker"
			quien["naturaleza"] = "Adamant"
			quien["objeto"] = "Leftovers"
		}
		status, datos := crearHTTP(t, srv, nuevoCliente(t), cuerpo)
		if status != http.StatusAccepted {
			t.Fatalf("%s: status %d %s", gen, status, datos)
		}
		id := int64(datos["id"].(float64))
		job, _ := a.repo.Get(context.Background(), id)
		var p Peticion
		if err := json.Unmarshal(job.Peticion, &p); err != nil {
			t.Fatal(err)
		}
		for _, pk := range append([]PokemonJSON{p.Rival}, p.Candidatos...) {
			if pk.Habilidad != "" || pk.Naturaleza != "" {
				t.Fatalf("%s: no se limpiaron habilidad/naturaleza: %+v", gen, pk)
			}
			if esperaObjeto && pk.Objeto == "" {
				t.Fatalf("%s: se perdió el objeto: %+v", gen, pk)
			}
			if !esperaObjeto && pk.Objeto != "" {
				t.Fatalf("%s: no se limpió el objeto: %+v", gen, pk)
			}
		}
	}

	comprobar("gen1", false)
	comprobar("gen2", true)
}

func TestDescartaHabilidadInvalida(t *testing.T) {
	a := nuevaAPI(t, nil)
	a.catalog.configurar(true, false)
	srv := a.servidor(t)

	status, datos := crearHTTP(t, srv, nuevoCliente(t), cuerpoValido())
	if status != http.StatusAccepted {
		t.Fatalf("status %d %s", status, datos)
	}
	id := int64(datos["id"].(float64))
	job, _ := a.repo.Get(context.Background(), id)
	var p Peticion
	if err := json.Unmarshal(job.Peticion, &p); err != nil {
		t.Fatal(err)
	}
	if p.Rival.Habilidad != "" {
		t.Fatalf("la habilidad inválida debería haberse descartado: %+v", p.Rival)
	}
	if a.catalog.numLlamadas() == 0 {
		t.Fatal("no se consultó el catálogo")
	}
}

func TestRechazaCamposDesconocidosPorHTTP(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cuerpo := cuerpoValido()
	cuerpo["foo"] = "bar"
	status, datos, _ := hacerJSON(t, nuevoCliente(t), http.MethodPost, srv.URL+"/api/simulaciones", cuerpo)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d", status)
	}
	if decodificar(t, datos)["error"] == nil {
		t.Fatalf("falta el mensaje de error: %s", datos)
	}
}

func TestServiceRechazaMaxTurnosInvalido(t *testing.T) {
	a := nuevaAPI(t, nil)
	p := peticionDeCuerpo(t, cuerpoValido())
	p.Opciones = &Opciones{MaxTurnos: intPtr(1001)}
	_, err := a.svc.Create(context.Background(), "u", p)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	var v *ValidationError
	if !errors.As(err, &v) || v.Campo != "opciones.maxTurnos" {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestInternoTokenY404(t *testing.T) {
	a := nuevaAPI(t, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	interno := httptest.NewServer(NewInternalHandler(a.svc, a.rt, a.cfg.TokenInterno, logger))
	t.Cleanup(interno.Close)

	req, _ := http.NewRequest(http.MethodPost, interno.URL+"/interno/simulaciones/1/lista", nil)
	req.Header.Set("Authorization", "Bearer incorrecto")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token incorrecto: %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, interno.URL+"/interno/simulaciones/999/lista", nil)
	req2.Header.Set("Authorization", "Bearer "+a.cfg.TokenInterno)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("id inexistente: %d", resp2.StatusCode)
	}
}

func TestInternoPendiente204(t *testing.T) {
	a := nuevaAPI(t, nil)
	job := a.repo.addJob(Job{Hash: "h", Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	interno := httptest.NewServer(NewInternalHandler(a.svc, a.rt, a.cfg.TokenInterno, logger))
	t.Cleanup(interno.Close)

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/interno/simulaciones/%d/lista", interno.URL, job.ID), nil)
	req.Header.Set("Authorization", "Bearer "+a.cfg.TokenInterno)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("callback pendiente: %d", resp.StatusCode)
	}
}
