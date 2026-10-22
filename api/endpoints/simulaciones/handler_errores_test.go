package simulaciones

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"PokeTeamCalculator/database"
)

func TestRutasConIDInvalido(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)
	for _, id := range []string{"abc", "0", "-3", "1.5"} {
		for _, peticion := range []struct{ metodo, ruta string }{
			{http.MethodGet, "/api/simulaciones/" + id},
			{http.MethodDelete, "/api/simulaciones/" + id},
			{http.MethodGet, "/api/simulaciones/" + id + "/eventos"},
		} {
			status, datos := hacerSinCuerpo(t, cliente, peticion.metodo, srv.URL+peticion.ruta)
			if status != http.StatusNotFound || decodificar(t, datos)["error"] != "Simulación no encontrada" {
				t.Fatalf("%s %s: %d %s", peticion.metodo, peticion.ruta, status, datos)
			}
		}
	}
}

func TestErroresDeLaBaseEnElHandler(t *testing.T) {
	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)
	job := a.repo.addJob(Job{Usuario: "u", Estado: EstadoCompletado, Resultado: []byte(`{}`)})
	ruta := srv.URL + "/api/simulaciones/" + strconv.FormatInt(job.ID, 10)

	a.repo.fallarEn("Get", errBase)
	for _, r := range []string{ruta, ruta + "/eventos"} {
		status, datos := hacerSinCuerpo(t, cliente, http.MethodGet, r)
		if status != http.StatusInternalServerError || decodificar(t, datos)["error"] != "No se pudo leer la simulación" {
			t.Fatalf("GET %s: %d %s", r, status, datos)
		}
	}

	a.repo.fallarEn("Cancel", errBase)
	status, datos := hacerSinCuerpo(t, cliente, http.MethodDelete, ruta)
	if status != http.StatusInternalServerError || decodificar(t, datos)["error"] != "No se pudo cancelar la simulación" {
		t.Fatalf("DELETE: %d %s", status, datos)
	}

	a.repo.fallarEn("CreateOrFind", errBase)
	status, datos, _ = hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido())
	if status != http.StatusInternalServerError || decodificar(t, datos)["error"] != "No se pudo crear la simulación" {
		t.Fatalf("POST: %d %s", status, datos)
	}
}

func TestCrearLimitePorMinuto(t *testing.T) {
	a := nuevaAPI(t, func(c *Config, _ *memRepo) { c.LimitePorMinuto = 1 })
	srv := a.servidor(t)
	cliente := nuevoCliente(t)
	if status, datos, _ := hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido()); status != http.StatusAccepted {
		t.Fatalf("primera: %d %s", status, datos)
	}
	status, datos, _ := hacerJSON(t, cliente, http.MethodPost, srv.URL+"/api/simulaciones", cuerpoValido())
	if status != http.StatusTooManyRequests || decodificar(t, datos)["error"] != "Demasiadas peticiones, espera un minuto" {
		t.Fatalf("segunda: %d %s", status, datos)
	}
	if a.repo.count() != 1 {
		t.Fatalf("la limitada no debe llegar a la base: %d trabajos", a.repo.count())
	}
}

func TestSalud(t *testing.T) {
	anterior := database.GetStatus()
	t.Cleanup(func() { database.SetStatus(anterior) })

	a := nuevaAPI(t, nil)
	srv := a.servidor(t)
	cliente := nuevoCliente(t)

	database.SetStatus("ok")
	status, datos := hacerSinCuerpo(t, cliente, http.MethodGet, srv.URL+"/api/salud")
	cuerpo := decodificar(t, datos)
	if status != http.StatusOK || cuerpo["status"] != "ok" || cuerpo["baseDatos"] != "ok" || cuerpo["cola"] != "ok" || cuerpo["workerDisponible"] != true {
		t.Fatalf("todo bien: %d %v", status, cuerpo)
	}

	database.SetStatus("error")
	status, datos = hacerSinCuerpo(t, cliente, http.MethodGet, srv.URL+"/api/salud")
	if status != http.StatusServiceUnavailable || decodificar(t, datos)["baseDatos"] != "error" {
		t.Fatalf("sin base: %d %s", status, datos)
	}

	// Sin Redis la API sigue sirviendo: solo lo indica.
	database.SetStatus("ok")
	a.sinWorkers(t)
	a.mr.Close()
	status, datos = hacerSinCuerpo(t, cliente, http.MethodGet, srv.URL+"/api/salud")
	cuerpo = decodificar(t, datos)
	if status != http.StatusOK || cuerpo["cola"] != "error" || cuerpo["workerDisponible"] != false {
		t.Fatalf("sin Redis: %d %v", status, cuerpo)
	}
}

func TestEventoInicial(t *testing.T) {
	casos := []struct {
		vista  JobView
		evento string
		final  bool
		estado string
		pos    int
	}{
		{JobView{Estado: EstadoPendiente, Posicion: 2, EstimadoSegundos: 9}, "posicion", false, EstadoPendiente, 2},
		{JobView{Estado: EstadoProcesando}, "procesando", false, EstadoProcesando, 0},
		{JobView{Estado: EstadoCompletado, Resultado: []byte(`{}`)}, "completado", true, EstadoCompletado, 0},
		{JobView{Estado: EstadoError, Error: "x"}, "error", true, EstadoError, 0},
		{JobView{Estado: EstadoCancelado}, "cancelado", true, EstadoCancelado, 0},
		{JobView{Estado: "raro"}, "cancelado", true, EstadoCancelado, 0},
	}
	for _, caso := range casos {
		ev, final, estado, pos := eventoInicial(&caso.vista)
		if ev.Name != caso.evento || final != caso.final || estado != caso.estado || pos != caso.pos {
			t.Errorf("%s: %s %v %s %d", caso.vista.Estado, ev.Name, final, estado, pos)
		}
	}
}

type escritorRoto struct{ httptest.ResponseRecorder }

func (escritorRoto) Write([]byte) (int, error) { return 0, errors.New("conexión cerrada") }

func TestEscribirEvento(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := escribirEvento(rec, Event{Name: "posicion", Data: []byte(`{"posicion":1}`)}); err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != "event: posicion\ndata: {\"posicion\":1}\n\n" || !rec.Flushed {
		t.Fatalf("%q flushed=%v", rec.Body.String(), rec.Flushed)
	}
	if err := escribirEvento(&escritorRoto{}, eventoProcesando()); err == nil {
		t.Fatal("se esperaba el error de escritura")
	}
}

func TestDtoDeVista(t *testing.T) {
	dto := dtoDeVista(&JobView{ID: 1, Estado: EstadoProcesando})
	if dto.Posicion != nil || dto.EstimadoSegundos != nil || dto.Resultado != nil || dto.Error != "" {
		t.Fatalf("%+v", dto)
	}
	dto = dtoDeVista(&JobView{ID: 1, Estado: EstadoPendiente, Posicion: 3, EstimadoSegundos: 12})
	if *dto.Posicion != 3 || *dto.EstimadoSegundos != 12 {
		t.Fatalf("%+v", dto)
	}
}

func TestNewHandlerSinLogger(t *testing.T) {
	h := NewHandler(nil, nil, configPrueba(), nil)
	if h.logger == nil || h.RateLimiter() == nil || h.RateLimiter() != h.limiter {
		t.Fatal("logger o limitador")
	}
}

func TestSesionesCookiesMalFormadas(t *testing.T) {
	s := NewSessions([]byte("0123456789abcdef0123456789abcdef"))
	sid := nuevoSID()
	casos := map[string]string{
		"tres partes":        sid + "." + s.firma(sid) + ".x",
		"sid corto":          "abc." + s.firma("abc"),
		"sid no hexadecimal": strings.Repeat("z", 32) + "." + s.firma(strings.Repeat("z", 32)),
	}
	for nombre, valor := range casos {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: nombreCookieSID, Value: valor})
		if _, ok := s.leer(req); ok {
			t.Errorf("%s: no debería aceptarse", nombre)
		}
	}
	if len(sid) != 32 || nuevoSID() == sid {
		t.Fatal("nuevoSID: 32 caracteres y distintos")
	}
}

func TestReglasDeGeneracion(t *testing.T) {
	for _, gen := range []string{"", "gen", "gen0", "gen10", "Gen4", "genX"} {
		if generacionDe(gen) != 0 {
			t.Errorf("%q no es una generación", gen)
		}
	}
	p := Peticion{Gen: "gen10", Rival: PokemonJSON{Habilidad: "Blaze", Naturaleza: "Adamant", Objeto: "Oran Berry"}}
	SanearPorGeneracion(&p)
	if p.Rival.Habilidad == "" || p.Rival.Objeto == "" {
		t.Fatal("con una generación inválida no se toca nada")
	}

	alto, bajo := 1.5, -0.1
	turnos0, turnos1001, turnos10 := 0, 1001, 10
	casos := []struct {
		op    *Opciones
		campo string
	}{
		{nil, ""},
		{&Opciones{}, ""},
		{&Opciones{AleatoriedadRival: &alto}, "opciones.aleatoriedadRival"},
		{&Opciones{AleatoriedadRival: &bajo}, "opciones.aleatoriedadRival"},
		{&Opciones{MaxTurnos: &turnos0}, "opciones.maxTurnos"},
		{&Opciones{MaxTurnos: &turnos1001}, "opciones.maxTurnos"},
		{&Opciones{MaxTurnos: &turnos10}, ""},
	}
	for _, caso := range casos {
		err := validarOpciones(caso.op)
		var v *ValidationError
		if caso.campo == "" && err != nil || caso.campo != "" && (!errors.As(err, &v) || v.Campo != caso.campo) {
			t.Errorf("%+v: %v", caso.op, err)
		}
	}
}

func TestRevisarHabilidades(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()

	// Antes de gen 3 no hay habilidades que revisar.
	p := Peticion{Gen: "gen2", Rival: PokemonJSON{Especie: "Onix", Habilidad: "Sturdy"}}
	a.svc.revisarHabilidades(ctx, &p)
	if a.catalog.numLlamadas() != 0 {
		t.Fatal("no debería consultar el catálogo en gen 2")
	}

	// Si el catálogo falla, se deja la habilidad: el worker decide.
	a.catalog.mu.Lock()
	a.catalog.err = errBase
	a.catalog.mu.Unlock()
	p = Peticion{Gen: "gen4", Rival: PokemonJSON{Especie: "Onix", Habilidad: "Levitate"}, Candidatos: []PokemonJSON{{Especie: "Geodude"}}}
	a.svc.revisarHabilidades(ctx, &p)
	if p.Rival.Habilidad != "Levitate" || a.catalog.numLlamadas() != 1 {
		t.Fatalf("habilidad %q, llamadas %d", p.Rival.Habilidad, a.catalog.numLlamadas())
	}

	// Sin catálogo no se revisa nada.
	sinCatalogo := NewService(a.repo, a.cfg, a.cola, nil)
	sinCatalogo.revisarHabilidades(ctx, &p)
}

func TestServiceCreateRechazaGenInvalida(t *testing.T) {
	a := nuevaAPI(t, nil)
	_, err := a.svc.Create(context.Background(), "u", Peticion{Gen: "gen0"})
	var v *ValidationError
	if !errors.As(err, &v) || v.Campo != "gen" {
		t.Fatalf("error %v", err)
	}
}

func TestValidationErrorSinCampo(t *testing.T) {
	if got := (&ValidationError{Mensaje: "algo"}).Error(); got != "algo" {
		t.Fatalf("%q", got)
	}
}
