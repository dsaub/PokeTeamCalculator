package simulaciones

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestMySQLCancelIfPending(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	pendiente := insertarPendiente(t, db, "c", 0)
	ok, err := repo.CancelIfPending(ctx, pendiente)
	if err != nil || !ok {
		t.Fatalf("pendiente: ok=%v err=%v", ok, err)
	}
	job, err := repo.Get(ctx, pendiente)
	if err != nil || job.Estado != EstadoCancelado || !job.Terminado.Valid {
		t.Fatalf("tras cancelar: %+v %v", job, err)
	}

	enProceso := insertarPendiente(t, db, "d", 0)
	if _, err := repo.MarcarProcesando(ctx, enProceso, "w1", 1); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.CancelIfPending(ctx, enProceso); err != nil || ok {
		t.Fatalf("en proceso: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.CancelIfPending(ctx, 999_999); err != nil || ok {
		t.Fatalf("inexistente: ok=%v err=%v", ok, err)
	}
}

func TestMySQLListAbandonedPending(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	viejo := insertarPendiente(t, db, "e", 600)
	_ = insertarPendiente(t, db, "f", 0)
	procesando := insertarPendiente(t, db, "g", 600)
	if _, err := repo.MarcarProcesando(ctx, procesando, "w1", 1); err != nil {
		t.Fatal(err)
	}

	ids, err := repo.ListAbandonedPending(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []int64{viejo}) {
		t.Fatalf("abandonados %v, esperado [%d]", ids, viejo)
	}
	vacio, err := repo.ListAbandonedPending(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(vacio) != 0 {
		t.Fatalf("sin abandonados: %v %v", vacio, err)
	}
}

func TestMySQLCleanup(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	insertar := func(hash, estado string, haceHoras int) int64 {
		t.Helper()
		id := insertarPendiente(t, db, hash, 0)
		if _, err := db.Exec(`UPDATE simulaciones SET estado = ?, terminado = NOW(3) - INTERVAL ? HOUR WHERE id = ?`,
			estado, haceHoras, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	errorViejo := insertar("h", EstadoError, 8*24)
	canceladoViejo := insertar("i", EstadoCancelado, 8*24)
	errorReciente := insertar("j", EstadoError, 24)
	completadoViejo := insertar("k", EstadoCompletado, 48)
	completadoReciente := insertar("l", EstadoCompletado, 1)
	pendienteViejo := insertarPendiente(t, db, "m", 30*24*3600)

	counts, err := repo.Cleanup(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (CleanupCounts{Errores: 2, Completados: 1}) {
		t.Fatalf("borrados %+v", counts)
	}
	for _, id := range []int64{errorViejo, canceladoViejo, completadoViejo} {
		if _, err := repo.Get(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%d debería haberse borrado: %v", id, err)
		}
	}
	for _, id := range []int64{errorReciente, completadoReciente, pendienteViejo} {
		if _, err := repo.Get(ctx, id); err != nil {
			t.Fatalf("%d no debería haberse borrado: %v", id, err)
		}
	}
}

func TestMySQLCreateOrFindLimiteGlobal(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()
	_ = insertarPendiente(t, db, "n", 0)

	out, err := repo.CreateOrFind(ctx, CreateInput{
		Hash: "z", Usuario: "otro", Peticion: []byte(`{}`), CacheDesde: time.Now().Add(-time.Hour), MaxPendientes: 5, MaxCola: 1,
	})
	if err != nil || !out.LimitGlobal || out.Job != nil {
		t.Fatalf("cola llena: %+v %v", out, err)
	}
}
