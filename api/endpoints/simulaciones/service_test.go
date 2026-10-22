package simulaciones

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

var errBase = errors.New("mariadb caída")

func TestServiceGetByIDYGetJob(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	job := a.repo.addJob(Job{Usuario: "u", Estado: EstadoError, Error: sql.NullString{String: "falló", Valid: true}})

	v, err := a.svc.GetByID(ctx, job.ID)
	if err != nil || v.ID != job.ID || v.Estado != EstadoError || v.Error != "falló" {
		t.Fatalf("GetByID: %+v %v", v, err)
	}
	crudo, err := a.svc.GetJob(ctx, job.ID)
	if err != nil || crudo.Usuario != "u" {
		t.Fatalf("GetJob: %+v %v", crudo, err)
	}

	for nombre, fn := range map[string]func() error{
		"GetByID":    func() error { _, err := a.svc.GetByID(ctx, 999); return err },
		"GetJob":     func() error { _, err := a.svc.GetJob(ctx, 999); return err },
		"GetForUser": func() error { _, err := a.svc.GetForUser(ctx, 999, "u"); return err },
	} {
		if err := fn(); !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("%s de un id inexistente: %v", nombre, err)
		}
	}

	a.repo.fallarEn("Get", errBase)
	for nombre, fn := range map[string]func() error{
		"GetByID":    func() error { _, err := a.svc.GetByID(ctx, job.ID); return err },
		"GetJob":     func() error { _, err := a.svc.GetJob(ctx, job.ID); return err },
		"GetForUser": func() error { _, err := a.svc.GetForUser(ctx, job.ID, "u"); return err },
	} {
		if err := fn(); !errors.Is(err, errBase) {
			t.Fatalf("%s con la base caída: %v", nombre, err)
		}
	}
}

func TestServiceCancelIfPendingLoSacaDeLaCola(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	pendiente := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente, Peticion: []byte(`{}`)})
	if _, err := a.cola.Encolar(ctx, pendiente.ID, pendiente.Peticion, 0); err != nil {
		t.Fatal(err)
	}

	ok, err := a.svc.CancelIfPending(ctx, pendiente.ID)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if a.repo.estado(pendiente.ID) != EstadoCancelado || len(a.enCola(t)) != 0 {
		t.Fatalf("estado %s, cola %v", a.repo.estado(pendiente.ID), a.enCola(t))
	}
	if cancelado, _ := a.rdb.HGet(ctx, claveTrabajo(pendiente.ID), "cancelado").Result(); cancelado != "1" {
		t.Fatalf("falta la lápida: %q", cancelado)
	}

	procesando := a.repo.addJob(Job{Usuario: "u", Estado: EstadoProcesando})
	if ok, err := a.svc.CancelIfPending(ctx, procesando.ID); ok || err != nil {
		t.Fatalf("procesando: ok=%v err=%v", ok, err)
	}

	a.repo.fallarEn("CancelIfPending", errBase)
	otro := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente})
	if ok, err := a.svc.CancelIfPending(ctx, otro.ID); ok || !errors.Is(err, errBase) {
		t.Fatalf("con error: ok=%v err=%v", ok, err)
	}
}

func TestServiceCancelarConRedisCaido(t *testing.T) {
	a := nuevaAPI(t, nil)
	job := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente})
	a.mr.Close()
	// MariaDB es la fuente de verdad: la cancelación vale aunque Redis no responda.
	if err := a.svc.Cancel(context.Background(), job.ID, "u"); err != nil {
		t.Fatal(err)
	}
	if a.repo.estado(job.ID) != EstadoCancelado {
		t.Fatalf("estado %s", a.repo.estado(job.ID))
	}
}

func TestServiceCancelPropagaElErrorDeLaBase(t *testing.T) {
	a := nuevaAPI(t, nil)
	a.repo.fallarEn("Cancel", errBase)
	if err := a.svc.Cancel(context.Background(), 1, "u"); !errors.Is(err, errBase) {
		t.Fatalf("error %v", err)
	}
}

func TestServiceAplicarEstadoTipoDesconocido(t *testing.T) {
	a := nuevaAPI(t, nil)
	if ok, err := a.svc.AplicarEstado(context.Background(), EventoWorker{ID: 1, Tipo: "raro"}); ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestServiceCleanupYAbandonados(t *testing.T) {
	a := nuevaAPI(t, func(c *Config, r *memRepo) {
		c.Retencion = 48 * time.Hour
		r.limpieza = CleanupCounts{Errores: 2, Completados: 3}
	})
	counts, err := a.svc.Cleanup(context.Background())
	if err != nil || counts != (CleanupCounts{Errores: 2, Completados: 3}) || a.repo.retencion != 48*time.Hour {
		t.Fatalf("Cleanup: %+v %v (retención %v)", counts, err, a.repo.retencion)
	}

	viejo := a.repo.addJob(Job{Estado: EstadoPendiente})
	a.repo.envejecer(viejo.ID, time.Hour)
	_ = a.repo.addJob(Job{Estado: EstadoPendiente})
	ids, err := a.svc.AbandonedPending(context.Background(), time.Now().Add(-time.Minute))
	if err != nil || len(ids) != 1 || ids[0] != viejo.ID {
		t.Fatalf("abandonados %v %v", ids, err)
	}
}

func TestServicePosicionSinRedisEsLaPrimera(t *testing.T) {
	a := nuevaAPI(t, nil)
	job := a.repo.addJob(Job{Usuario: "u", Estado: EstadoPendiente})
	a.mr.Close()
	v, err := a.svc.GetForUser(context.Background(), job.ID, "u")
	if err != nil || v.Posicion != 1 || v.EstimadoSegundos <= 0 {
		t.Fatalf("vista %+v %v", v, err)
	}
	if a.svc.ColaResponde(context.Background()) {
		t.Fatal("Redis está cerrado")
	}
}

func TestServiceColaResponde(t *testing.T) {
	a := nuevaAPI(t, nil)
	if !a.svc.ColaResponde(context.Background()) {
		t.Fatal("Redis está arriba")
	}
}

func TestMediaDuraciones(t *testing.T) {
	casos := []struct {
		nombre     string
		duraciones []time.Duration
		media      time.Duration
	}{
		{"sin datos", nil, 10 * time.Second},
		{"una", []time.Duration{4 * time.Second}, 4 * time.Second},
		{"varias", []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second}, 4 * time.Second},
		{"media no positiva", []time.Duration{0, -time.Second}, 10 * time.Second},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := mediaDuraciones(caso.duraciones); got != caso.media {
				t.Fatalf("media %v, esperada %v", got, caso.media)
			}
		})
	}
}

func TestEstimador(t *testing.T) {
	t.Run("posición 0 o negativa → 0 sin consultar", func(t *testing.T) {
		repo := newMemRepo()
		e := NewEstimator(repo, 2)
		if e.Seconds(context.Background(), 0) != 0 || e.Seconds(context.Background(), -1) != 0 {
			t.Fatal("se esperaba 0")
		}
		if repo.numLlamadas("LastDurations") != 0 {
			t.Fatal("no debería consultar la base")
		}
	})

	t.Run("posición × media / simultáneos, redondeando hacia arriba", func(t *testing.T) {
		repo := newMemRepo()
		repo.duraciones = []time.Duration{3 * time.Second, 4 * time.Second}
		e := NewEstimator(repo, 2)
		if s := e.Seconds(context.Background(), 3); s != 6 { // 3 × 3,5 / 2 = 5,25 → 6
			t.Fatalf("segundos %d", s)
		}
	})

	t.Run("guarda la media 30 s", func(t *testing.T) {
		repo := newMemRepo()
		e := NewEstimator(repo, 1)
		for i := 0; i < 5; i++ {
			e.Seconds(context.Background(), 1)
		}
		if n := repo.numLlamadas("LastDurations"); n != 1 {
			t.Fatalf("consultas %d", n)
		}
		e.ultimoCalc = time.Now().Add(-31 * time.Second)
		e.Seconds(context.Background(), 1)
		if n := repo.numLlamadas("LastDurations"); n != 2 {
			t.Fatalf("tras caducar: %d", n)
		}
	})

	t.Run("si la base falla usa 10 s y no reintenta en cada petición", func(t *testing.T) {
		repo := newMemRepo()
		repo.fallarEn("LastDurations", errBase)
		e := NewEstimator(repo, 1)
		if s := e.Seconds(context.Background(), 2); s != 20 {
			t.Fatalf("segundos %d", s)
		}
		e.Seconds(context.Background(), 2)
		if n := repo.numLlamadas("LastDurations"); n != 1 {
			t.Fatalf("consultas %d", n)
		}
	})

	t.Run("menos de un simultáneo cuenta como uno", func(t *testing.T) {
		e := NewEstimator(newMemRepo(), 0)
		if e.simultaneos != 1 || e.Seconds(context.Background(), 1) != 10 {
			t.Fatalf("simultáneos %d", e.simultaneos)
		}
	})

	t.Run("media a cero en memoria usa 10 s", func(t *testing.T) {
		e := NewEstimator(newMemRepo(), 1)
		e.ultimoCalc = time.Now()
		e.media = 0
		if s := e.Seconds(context.Background(), 1); s != 10 {
			t.Fatalf("segundos %d", s)
		}
	})
}

func TestNewServiceSinLoggerNiCatalogo(t *testing.T) {
	svc := NewService(newMemRepo(), configPrueba(), nil, nil)
	if svc.logger == nil || svc.catalog != nil {
		t.Fatalf("logger %v, catálogo %v", svc.logger, svc.catalog)
	}
}

func TestLimiteUsuarioError(t *testing.T) {
	err := error(&LimiteUsuarioError{ID: 7})
	if err.Error() != "Ya tienes una simulación en cola" || !errors.Is(err, ErrLimiteUsuario) {
		t.Fatalf("%v", err)
	}
	var limite *LimiteUsuarioError
	if !errors.As(err, &limite) || limite.ID != 7 {
		t.Fatalf("%+v", limite)
	}
}
