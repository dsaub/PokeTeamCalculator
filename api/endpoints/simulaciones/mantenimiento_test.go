package simulaciones

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// ctxCancelado sirve para comprobar que los bucles hacen su primera pasada y
// terminan al cancelar el contexto.
func ctxCancelado() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func terminaPronto(t *testing.T, nombre string, fn func()) {
	t.Helper()
	hecho := make(chan struct{})
	go func() {
		fn()
		close(hecho)
	}()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s no terminó al cancelar el contexto", nombre)
	}
}

func TestCancelarAbandonados(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	espera := 2 * time.Minute
	nuevoPendiente := func(edad time.Duration) int64 {
		j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
		a.repo.envejecer(j.ID, edad)
		if _, err := a.cola.Encolar(ctx, j.ID, j.Peticion, 0); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}

	nuncaAbierto := nuevoPendiente(time.Hour)
	reciente := nuevoPendiente(time.Second)
	conCliente := nuevoPendiente(time.Hour)
	sub := a.hub.Subscribe(conCliente)
	defer a.hub.Unsubscribe(conCliente, sub)
	reciénCerrado := nuevoPendiente(time.Hour)
	a.hub.Unsubscribe(reciénCerrado, a.hub.Subscribe(reciénCerrado))
	cerradoHaceTiempo := nuevoPendiente(time.Hour)
	a.hub.Unsubscribe(cerradoHaceTiempo, a.hub.Subscribe(cerradoHaceTiempo))
	a.hub.mu.Lock()
	a.hub.sinGente[cerradoHaceTiempo] = time.Now().Add(-time.Hour)
	a.hub.mu.Unlock()

	a.rt.cancelarAbandonados(ctx, espera)

	esperado := map[int64]string{
		nuncaAbierto:      EstadoCancelado,
		reciente:          EstadoPendiente,
		conCliente:        EstadoPendiente,
		reciénCerrado:     EstadoPendiente,
		cerradoHaceTiempo: EstadoCancelado,
	}
	for id, estado := range esperado {
		if got := a.repo.estado(id); got != estado {
			t.Errorf("trabajo %d: %s, esperado %s", id, got, estado)
		}
	}
	cola := a.enCola(t)
	if len(cola) != 3 {
		t.Fatalf("cola %v", cola)
	}
}

func TestCancelarAbandonadosConErrores(t *testing.T) {
	a := nuevaAPI(t, nil)
	j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente})
	a.repo.envejecer(j.ID, time.Hour)

	a.repo.fallarEn("ListAbandonedPending", errBase)
	a.rt.cancelarAbandonados(context.Background(), time.Minute)
	if a.repo.numLlamadas("CancelIfPending") != 0 {
		t.Fatal("sin la lista no debería cancelar nada")
	}

	a.repo.fallarEn("ListAbandonedPending", nil)
	a.repo.fallarEn("CancelIfPending", errBase)
	a.rt.cancelarAbandonados(context.Background(), time.Minute)
	if a.repo.estado(j.ID) != EstadoPendiente || a.repo.numLlamadas("CancelIfPending") != 1 {
		t.Fatalf("estado %s", a.repo.estado(j.ID))
	}
}

func TestCancelAbandonedLoop(t *testing.T) {
	a := nuevaAPI(t, func(c *Config, _ *memRepo) { c.CancelarSinCliente = 0 })
	// Desactivado: vuelve sin esperar al contexto.
	terminaPronto(t, "CancelAbandonedLoop desactivado", func() { a.rt.CancelAbandonedLoop(context.Background()) })

	for _, espera := range []time.Duration{time.Millisecond, time.Minute, time.Hour} {
		b := nuevaAPI(t, func(c *Config, _ *memRepo) { c.CancelarSinCliente = espera })
		terminaPronto(t, "CancelAbandonedLoop", func() { b.rt.CancelAbandonedLoop(ctxCancelado()) })
	}
}

func TestLimpiar(t *testing.T) {
	a := nuevaAPI(t, func(_ *Config, r *memRepo) { r.limpieza = CleanupCounts{Errores: 1} })
	terminaPronto(t, "CleanupLoop", func() { a.rt.CleanupLoop(ctxCancelado()) })
	if n := a.repo.numLlamadas("Cleanup"); n != 1 {
		t.Fatalf("CleanupLoop limpia al arrancar: %d llamadas", n)
	}
	if a.repo.retencion != a.cfg.Retencion {
		t.Fatalf("retención %v", a.repo.retencion)
	}

	a.repo.limpieza = CleanupCounts{}
	a.rt.limpiar(context.Background())
	a.repo.fallarEn("Cleanup", errBase)
	a.rt.limpiar(context.Background())
	if n := a.repo.numLlamadas("Cleanup"); n != 3 {
		t.Fatalf("llamadas %d", n)
	}
}

func TestReconciliarLoopReconciliaAlArrancar(t *testing.T) {
	a := nuevaAPI(t, nil)
	j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	a.repo.envejecer(j.ID, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	hecho := make(chan struct{})
	go func() {
		a.rt.ReconciliarLoop(ctx)
		close(hecho)
	}()
	limite := time.Now().Add(2 * time.Second)
	for len(a.enCola(t)) == 0 {
		if time.Now().After(limite) {
			t.Fatal("no reconcilió al arrancar")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	terminaPronto(t, "ReconciliarLoop", func() { <-hecho })
	if cola := a.enCola(t); len(cola) != 1 || cola[0] != j.ID {
		t.Fatalf("cola %v", cola)
	}
}

func TestReconciliarConErrores(t *testing.T) {
	t.Run("Redis caído: no toca nada", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		j := a.repo.addJob(Job{Estado: EstadoPendiente})
		a.repo.envejecer(j.ID, time.Minute)
		a.mr.Close()
		a.rt.Reconciliar(context.Background())
		if a.repo.numLlamadas("ListActivos") != 0 {
			t.Fatal("no debería leer los activos sin Redis")
		}
	})

	t.Run("fallo al leer los activos", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		a.repo.fallarEn("ListActivos", errBase)
		a.rt.Reconciliar(context.Background())
		if a.repo.numLlamadas("EstadosDe") != 0 {
			t.Fatal("debería parar tras el error")
		}
	})

	t.Run("fallo al leer los estados de la cola", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		huerfano := a.repo.addJob(Job{Estado: EstadoCompletado, Peticion: []byte(`{}`)})
		if _, err := a.cola.Encolar(context.Background(), huerfano.ID, huerfano.Peticion, 0); err != nil {
			t.Fatal(err)
		}
		a.repo.fallarEn("EstadosDe", errBase)
		a.rt.Reconciliar(context.Background())
		if cola := a.enCola(t); len(cola) != 1 {
			t.Fatalf("sin estados no debe quitar nada de la cola: %v", cola)
		}
	})

	t.Run("fallo al marcar pendiente un trabajo reencolado", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		j := a.repo.addJob(Job{Estado: EstadoProcesando, Peticion: []byte(`{}`)})
		a.repo.envejecer(j.ID, time.Minute)
		a.repo.setErrEstados(errBase)
		a.rt.Reconciliar(context.Background())
		if cola := a.enCola(t); len(cola) != 1 || cola[0] != j.ID {
			t.Fatalf("se reencola igual: %v", cola)
		}
		if a.repo.estado(j.ID) != EstadoProcesando {
			t.Fatalf("estado %s", a.repo.estado(j.ID))
		}
	})
}

func TestRunYPollOnce(t *testing.T) {
	a := nuevaAPI(t, nil)
	terminaPronto(t, "Run", func() { a.rt.Run(ctxCancelado()) })

	// Sin suscriptores no consulta la base.
	a.rt.PollOnce(context.Background())
	if a.repo.numLlamadas("GetMany") != 0 {
		t.Fatal("PollOnce sin suscriptores no debería consultar")
	}

	j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoCompletado, Resultado: []byte(`{"ok":true}`)})
	sub := a.hub.Subscribe(j.ID)
	a.repo.fallarEn("GetMany", errBase)
	a.rt.PollOnce(context.Background())
	sinEventos(t, sub)

	// Sin Redis no hay posiciones, pero el final se publica igual.
	a.repo.fallarEn("GetMany", nil)
	a.mr.Close()
	a.rt.PollOnce(context.Background())
	if ev := recibir(t, sub); ev.Name != "completado" || string(ev.Data) != `{"resultado":{"ok":true}}` {
		t.Fatalf("evento %s %s", ev.Name, ev.Data)
	}
}

func TestPublicarSinRedis(t *testing.T) {
	a := nuevaAPI(t, nil)
	j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente})
	sub := a.hub.Subscribe(j.ID)
	a.mr.Close()
	a.rt.PublishAllPositions(context.Background())
	sinEventos(t, sub)
	// PublishCancel cierra el SSE aunque no pueda avisar a las demás instancias.
	a.rt.PublishCancel(context.Background(), j.ID)
	if ev := recibir(t, sub); ev.Name != "cancelado" {
		t.Fatalf("evento %s", ev.Name)
	}
}

func TestRefrescarConErrores(t *testing.T) {
	a := nuevaAPI(t, nil)
	j := a.repo.addJob(Job{Usuario: "u", Estado: EstadoProcesando})
	sub := a.hub.Subscribe(j.ID)

	a.repo.fallarEn("Get", errBase)
	a.rt.Refrescar(context.Background(), j.ID)
	sinEventos(t, sub)

	// Un trabajo que ya no existe se ignora sin más.
	a.repo.fallarEn("Get", nil)
	otro := a.hub.Subscribe(999)
	a.rt.Refrescar(context.Background(), 999)
	sinEventos(t, otro)

	a.rt.Refrescar(context.Background(), j.ID)
	if ev := recibir(t, sub); ev.Name != "procesando" {
		t.Fatalf("evento %s", ev.Name)
	}
}

func TestEventosFinales(t *testing.T) {
	casos := []struct {
		nombre string
		job    Job
		evento string
		datos  string
		final  bool
	}{
		{"completado", Job{Estado: EstadoCompletado, Resultado: []byte(`{"a":1}`)}, "completado", `{"resultado":{"a":1}}`, true},
		{"completado sin resultado", Job{Estado: EstadoCompletado}, "completado", `{"resultado":null}`, true},
		{"completado con JSON roto", Job{Estado: EstadoCompletado, Resultado: []byte(`{roto`)}, "completado", `{"resultado":null}`, true},
		{"error", Job{Estado: EstadoError, Error: sql.NullString{String: `Falló "x"`, Valid: true}}, "error", `{"error":"Falló \"x\""}`, true},
		{"cancelado", Job{Estado: EstadoCancelado}, "cancelado", `{}`, true},
		{"pendiente no es final", Job{Estado: EstadoPendiente}, "", "", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			ev, ok := eventoFinal(&caso.job)
			if ok != caso.final || ev.Name != caso.evento || string(ev.Data) != caso.datos || ev.Final != caso.final {
				t.Fatalf("(%+v, %v) → %s %s", ev, ok, ev.Name, ev.Data)
			}
		})
	}
}

func TestNewRealtimeSinLogger(t *testing.T) {
	rt := NewRealtime(NewHub(), nil, nil, time.Second, nil)
	if rt.logger == nil || rt.Hub() == nil {
		t.Fatal("logger o hub nil")
	}
}

func TestAvisarSinRedis(t *testing.T) {
	a := nuevaAPI(t, nil)
	a.mr.Close()
	a.rt.avisar(context.Background(), 1)
	a.rt.Notificar(context.Background(), 1)
}

