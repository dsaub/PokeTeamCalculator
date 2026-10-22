package simulaciones

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func crearPendiente(t *testing.T, a *apiPrueba) int64 {
	t.Helper()
	job := a.repo.addJob(Job{Hash: "h", Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{"gen":"gen4"}`)})
	if _, err := a.cola.Encolar(context.Background(), job.ID, job.Peticion, 0); err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func TestEstadosAplicaCadaTipo(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	id := crearPendiente(t, a)

	a.tomar(t, "w1")
	a.aplicarEstados(t)
	if a.repo.estado(id) != EstadoProcesando || a.repo.workers[id] != "w1" || a.repo.intentos[id] != 1 {
		t.Fatalf("tras procesando: %s %q %d", a.repo.estado(id), a.repo.workers[id], a.repo.intentos[id])
	}

	a.reintentar(t, "w1", id)
	a.aplicarEstados(t)
	if a.repo.estado(id) != EstadoPendiente {
		t.Fatalf("tras el reintento: %s", a.repo.estado(id))
	}

	a.tomar(t, "w2")
	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 2, "worker", "w2", "resultado", `{"gen":"gen4"}`)
	a.aplicarEstados(t)
	job, _ := a.repo.Get(ctx, id)
	if job.Estado != EstadoCompletado || string(job.Resultado) != `{"gen":"gen4"}` {
		t.Fatalf("tras completado: %+v", job)
	}
	if n, _ := a.cola.EstadosSinAplicar(ctx); n != 0 {
		t.Fatalf("quedan %d estados en el stream", n)
	}

	otro := crearPendiente(t, a)
	a.tomar(t, "w1")
	a.publicarEstado(t, "id", otro, "tipo", EstadoError, "intentos", 1, "worker", "w1", "error", `Candidato 1: "Sylveon" no existe en gen 4.`)
	a.aplicarEstados(t)
	if job, _ := a.repo.Get(ctx, otro); job.Estado != EstadoError || job.Error.String != `Candidato 1: "Sylveon" no existe en gen 4.` {
		t.Fatalf("tras error: %+v", job)
	}
}

func TestEstadosNoPisanUnCancelado(t *testing.T) {
	a := nuevaAPI(t, nil)
	id := crearPendiente(t, a)
	a.tomar(t, "w1")
	a.aplicarEstados(t)
	if err := a.svc.Cancel(context.Background(), id, "u"); err != nil {
		t.Fatal(err)
	}

	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1", "resultado", `{}`)
	a.aplicarEstados(t)
	if a.repo.estado(id) != EstadoCancelado {
		t.Fatalf("el resultado pisó la cancelación: %s", a.repo.estado(id))
	}
	if n, _ := a.cola.EstadosSinAplicar(context.Background()); n != 0 {
		t.Fatalf("el estado ignorado debe confirmarse igual: quedan %d", n)
	}
}

func TestEstadosInvalidosSeConfirman(t *testing.T) {
	a := nuevaAPI(t, nil)
	id := crearPendiente(t, a)

	a.publicarEstado(t, "id", "no-es-un-id", "tipo", EstadoProcesando, "intentos", 1)
	a.publicarEstado(t, "id", id, "tipo", "desconocido", "intentos", 1)
	a.publicarEstado(t, "id", id, "tipo", EstadoProcesando)
	a.aplicarEstados(t)

	if a.repo.estado(id) != EstadoPendiente {
		t.Fatalf("un estado inválido cambió la fila: %s", a.repo.estado(id))
	}
	if n, _ := a.cola.EstadosSinAplicar(context.Background()); n != 0 {
		t.Fatalf("los estados inválidos no deben atascar el stream: quedan %d", n)
	}
}

func TestEstadosResultadoInvalidoPasaAError(t *testing.T) {
	a := nuevaAPI(t, nil)
	id := crearPendiente(t, a)
	a.tomar(t, "w1")
	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1", "resultado", `{"roto`)
	a.aplicarEstados(t)

	job, _ := a.repo.Get(context.Background(), id)
	if job.Estado != EstadoError || job.Error.String != mensajeErrorInterno {
		t.Fatalf("un resultado que no es JSON debe acabar en error: %+v", job)
	}
}

func TestEstadosFalloDeBaseDeDatosSeReintenta(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	id := crearPendiente(t, a)
	a.tomar(t, "w1")

	a.repo.setErrEstados(errors.New("MariaDB caída"))
	a.aplicarEstados(t)
	if n, _ := a.cola.EstadosSinAplicar(ctx); n != 1 {
		t.Fatalf("el estado no aplicado debe seguir en el stream: %d", n)
	}

	// Al volver MariaDB, el estado se reclama y se aplica.
	a.repo.setErrEstados(nil)
	mensajes, err := a.cola.ReclamarEstados(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	a.estados.aplicarTodos(ctx, mensajes)
	if a.repo.estado(id) != EstadoProcesando {
		t.Fatalf("tras reclamar: %s", a.repo.estado(id))
	}
	if n, _ := a.cola.EstadosSinAplicar(ctx); n != 0 {
		t.Fatalf("quedan %d estados", n)
	}
}

// Con varias réplicas de la API, el cambio que aplica una llega al SSE de las
// demás por Redis.
func TestAvisoEntreInstancias(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := crearPendiente(t, a)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rdbB := redis.NewClient(&redis.Options{Addr: a.mr.Addr()})
	t.Cleanup(func() { _ = rdbB.Close() })
	colaB := NewColaRedis(rdbB, a.cfg.LatidoCaducadoWorker, logger)
	svcB := NewService(a.repo, a.cfg, colaB, logger, a.catalog)
	hubB := NewHub()
	rtB := NewRealtime(hubB, svcB, colaB, a.cfg.IntervaloSondeoSSE, logger)
	sub := hubB.Subscribe(id)
	go rtB.Escuchar(ctx)
	for i := 0; len(a.mr.PubSubNumSub(canalAvisos)) == 0 || a.mr.PubSubNumSub(canalAvisos)[canalAvisos] == 0; i++ {
		if i > 200 {
			t.Fatal("la segunda instancia no se suscribió")
		}
		time.Sleep(5 * time.Millisecond)
	}

	a.tomar(t, "w1")
	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1", "resultado", `{"gen":"gen4"}`)
	a.aplicarEstados(t)

	// La otra instancia lee el estado actual al recibir cada aviso: puede
	// saltarse `procesando` si el final ya está aplicado, pero el final llega.
	for {
		select {
		case ev := <-sub.Ch:
			if ev.Name == "procesando" {
				continue
			}
			if ev.Name != "completado" {
				t.Fatalf("la otra instancia recibió %q", ev.Name)
			}
			return
		case <-time.After(3 * time.Second):
			t.Fatal("el aviso no llegó a la otra instancia")
		}
	}
}

func TestReconciliarEncolaLoQueFaltaEnRedis(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	pendiente := a.repo.addJob(Job{Hash: "a", Usuario: "u1", Estado: EstadoPendiente, Peticion: []byte(`{"a":1}`)})
	procesando := a.repo.addJob(Job{Hash: "b", Usuario: "u2", Estado: EstadoProcesando, Peticion: []byte(`{"b":2}`)})
	reciente := a.repo.addJob(Job{Hash: "c", Usuario: "u3", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	a.repo.envejecer(pendiente.ID, 2*time.Minute)
	a.repo.envejecer(procesando.ID, time.Minute)
	a.repo.intentos[procesando.ID] = 2

	a.rt.Reconciliar(ctx)

	// En orden de creación y al final; el recién creado lo encola su petición.
	if cola := a.enCola(t); len(cola) != 2 || cola[0] != pendiente.ID || cola[1] != procesando.ID {
		t.Fatalf("cola %v, se esperaba [%d %d]", cola, pendiente.ID, procesando.ID)
	}
	if a.mr.HGet(claveTrabajo(procesando.ID), "intentos") != "2" || a.mr.HGet(claveTrabajo(pendiente.ID), "peticion") != `{"a":1}` {
		t.Fatal("el hash no conserva la petición o los intentos")
	}
	// El que estaba en proceso ya no lo puede publicar nadie: vuelve a pendiente.
	if a.repo.estado(procesando.ID) != EstadoPendiente {
		t.Fatalf("estado %s", a.repo.estado(procesando.ID))
	}
	if a.mr.Exists(claveTrabajo(reciente.ID)) {
		t.Fatal("no debe tocar un trabajo recién creado")
	}

	// Es idempotente.
	a.rt.Reconciliar(ctx)
	if cola := a.enCola(t); len(cola) != 2 {
		t.Fatalf("la segunda pasada duplicó trabajos: %v", cola)
	}
}

func TestReconciliarQuitaHuerfanos(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	vivo := crearPendiente(t, a)
	terminado := crearPendiente(t, a)
	a.repo.setEstado(terminado, EstadoCompletado, []byte(`{}`), "")
	if _, err := a.cola.Encolar(ctx, 999, []byte(`{}`), 0); err != nil {
		t.Fatal(err)
	}

	a.rt.Reconciliar(ctx)
	if cola := a.enCola(t); len(cola) != 1 || cola[0] != vivo {
		t.Fatalf("cola %v, se esperaba [%d]", cola, vivo)
	}
}

func TestReconciliarEsperaALosEstados(t *testing.T) {
	a := nuevaAPI(t, nil)
	job := a.repo.addJob(Job{Hash: "a", Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	a.repo.envejecer(job.ID, time.Minute)
	a.publicarEstado(t, "id", job.ID, "tipo", EstadoProcesando, "intentos", 1, "worker", "w1")

	a.rt.Reconciliar(context.Background())
	if cola := a.enCola(t); len(cola) != 0 {
		t.Fatalf("no debe reconciliar con estados sin aplicar: %v", cola)
	}
}

func TestColaEncolarYCancelar(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()

	for i, id := range []int64{1, 2, 3} {
		pos, err := a.cola.Encolar(ctx, id, []byte(`{}`), 0)
		if err != nil || pos != i+1 {
			t.Fatalf("encolar %d: posición %d (%v)", id, pos, err)
		}
	}
	if pos, err := a.cola.Encolar(ctx, 2, []byte(`{}`), 0); err != nil || pos != 0 {
		t.Fatalf("encolar dos veces: %d (%v)", pos, err)
	}
	posiciones, err := a.cola.Posiciones(ctx)
	if err != nil || posiciones[1] != 1 || posiciones[2] != 2 || posiciones[3] != 3 {
		t.Fatalf("posiciones %v (%v)", posiciones, err)
	}
	if ttl := a.mr.TTL(claveTrabajo(1)); ttl <= 0 || ttl > ttlTrabajo {
		t.Fatalf("TTL del trabajo %v", ttl)
	}

	if err := a.cola.Cancelar(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if cola := a.enCola(t); len(cola) != 2 || cola[0] != 1 || cola[1] != 3 {
		t.Fatalf("cola tras cancelar %v", cola)
	}
	if a.mr.HGet(claveTrabajo(2), "cancelado") != "1" || a.mr.TTL(claveTrabajo(2)) > ttlLapida {
		t.Fatal("cancelar debe dejar la lápida con TTL de 1 h")
	}
	// La lápida impide volver a encolarlo.
	if pos, _ := a.cola.Encolar(ctx, 2, []byte(`{}`), 0); pos != 0 {
		t.Fatal("un cancelado no debe volver a la cola")
	}
}

func TestLeerEstado(t *testing.T) {
	largo := strings.Repeat("w", 100)
	casos := []struct {
		nombre string
		campos map[string]any
		error  string
		check  func(t *testing.T, ev EventoWorker)
	}{
		{"procesando", map[string]any{"id": "3", "tipo": EstadoProcesando, "intentos": "1", "worker": "w1"}, "", func(t *testing.T, ev EventoWorker) {
			if ev.ID != 3 || ev.Intentos != 1 || ev.Worker != "w1" || ev.Entrada != "1-0" {
				t.Fatalf("%+v", ev)
			}
		}},
		{"pendiente con motivo", map[string]any{"id": "3", "tipo": EstadoPendiente, "intentos": "0", "motivo": "devuelto"}, "", func(t *testing.T, ev EventoWorker) {
			if ev.Motivo != "devuelto" || ev.Intentos != 0 {
				t.Fatalf("%+v", ev)
			}
		}},
		{"error sin mensaje → mensaje interno", map[string]any{"id": "3", "tipo": EstadoError, "intentos": "3"}, "", func(t *testing.T, ev EventoWorker) {
			if ev.Error != mensajeErrorInterno {
				t.Fatalf("%q", ev.Error)
			}
		}},
		{"worker de más de 64 se recorta", map[string]any{"id": "3", "tipo": EstadoProcesando, "intentos": "1", "worker": largo}, "", func(t *testing.T, ev EventoWorker) {
			if ev.Worker != largo[:maxWorker] {
				t.Fatalf("%q", ev.Worker)
			}
		}},
		{"id cero", map[string]any{"id": "0", "tipo": EstadoProcesando, "intentos": "1"}, "id inválido", nil},
		{"sin id", map[string]any{"tipo": EstadoProcesando, "intentos": "1"}, "id inválido", nil},
		{"intentos negativos", map[string]any{"id": "3", "tipo": EstadoProcesando, "intentos": "-1"}, "intentos inválidos", nil},
		{"intentos no numéricos", map[string]any{"id": "3", "tipo": EstadoProcesando, "intentos": "x"}, "intentos inválidos", nil},
		{"tipo desconocido", map[string]any{"id": "3", "tipo": "cancelado", "intentos": "1"}, "tipo desconocido", nil},
		{"campo que no es texto", map[string]any{"id": 3, "tipo": EstadoProcesando, "intentos": "1"}, "id inválido", nil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			ev, err := leerEstado(redis.XMessage{ID: "1-0", Values: caso.campos})
			if caso.error != "" {
				if err == nil || !strings.Contains(err.Error(), caso.error) {
					t.Fatalf("error %v, esperado %q", err, caso.error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			caso.check(t, ev)
		})
	}
}

func TestResultadoValido(t *testing.T) {
	casos := map[string]struct {
		resultado []byte
		valido    bool
	}{
		"JSON":         {[]byte(`{"gen":"gen4"}`), true},
		"vacío":        {nil, false},
		"no JSON":      {[]byte(`{"a"`), false},
		"justo 4 MiB":  {append(append([]byte(`"`), []byte(strings.Repeat("a", maxResultado-2))...), '"'), true},
		"más de 4 MiB": {append(append([]byte(`"`), []byte(strings.Repeat("a", maxResultado-1))...), '"'), false},
	}
	for nombre, caso := range casos {
		if got := resultadoValido(caso.resultado); got != caso.valido {
			t.Errorf("%s: %v, esperado %v", nombre, got, caso.valido)
		}
	}
}

func TestEstadosResultadoVacioPasaAError(t *testing.T) {
	a := nuevaAPI(t, nil)
	id := crearPendiente(t, a)
	a.tomar(t, "w1")
	a.publicarEstado(t, "id", id, "tipo", EstadoCompletado, "intentos", 1, "worker", "w1")
	a.aplicarEstados(t)
	if job, _ := a.repo.Get(context.Background(), id); job.Estado != EstadoError || job.Error.String != mensajeErrorInterno {
		t.Fatalf("%+v", job)
	}
}

func TestEstadosRun(t *testing.T) {
	a := nuevaAPI(t, nil)
	id := crearPendiente(t, a)
	// Un estado que quedó sin confirmar de otra instancia: Run lo reclama al arrancar.
	a.tomar(t, "w1")
	if err := a.cola.prepararGrupo(context.Background()); err != nil {
		t.Fatal(err)
	}
	otra := NewColaRedis(a.rdb, time.Second, nil)
	if msgs, err := otra.LeerEstados(context.Background(), -1); err != nil || len(msgs) != 1 {
		t.Fatalf("%v %v", msgs, err)
	}
	a.mr.SetTime(time.Now().Add(2 * time.Minute))
	a.mr.FastForward(2 * time.Minute)

	a.estados.bloqueo = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	hecho := make(chan struct{})
	go func() {
		a.estados.Run(ctx)
		close(hecho)
	}()
	limite := time.Now().Add(3 * time.Second)
	for a.repo.estado(id) != EstadoProcesando {
		if time.Now().After(limite) {
			cancel()
			t.Fatalf("Run no aplicó el estado reclamado: %s", a.repo.estado(id))
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatal("Run no terminó al cancelar el contexto")
	}
}

func TestEstadosRunSinRedisTerminaConElContexto(t *testing.T) {
	a := nuevaAPI(t, nil)
	a.mr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	terminaPronto(t, "Estados.Run", func() { a.estados.Run(ctx) })
}

func TestDormir(t *testing.T) {
	if !dormir(context.Background(), time.Millisecond) {
		t.Fatal("sin cancelar debería devolver true")
	}
	if dormir(ctxCancelado(), time.Hour) {
		t.Fatal("con el contexto cancelado debería devolver false")
	}
}

func TestNewEstadosSinLogger(t *testing.T) {
	if e := NewEstados(nil, nil, nil, nil); e.logger == nil || e.bloqueo != bloqueoEstados {
		t.Fatalf("logger %v, bloqueo %v", e.logger, e.bloqueo)
	}
}
