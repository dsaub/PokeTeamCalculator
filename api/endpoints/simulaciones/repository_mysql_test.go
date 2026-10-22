package simulaciones

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const esquemaPruebas = `
CREATE TABLE IF NOT EXISTS simulaciones (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  hash       CHAR(64) NOT NULL,
  usuario    VARCHAR(64) NOT NULL,
  peticion   JSON NOT NULL,
  estado     ENUM('pendiente','procesando','completado','error','cancelado') NOT NULL DEFAULT 'pendiente',
  resultado  JSON NULL,
  error      TEXT NULL,
  intentos   INT NOT NULL DEFAULT 0,
  worker     VARCHAR(64) NULL,
  creado     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  iniciado   DATETIME(3) NULL,
  latido     DATETIME(3) NULL,
  terminado  DATETIME(3) NULL,
  INDEX idx_cola (estado, creado),
  INDEX idx_hash (hash)
) ENGINE=InnoDB`

func abrirBDPruebas(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_DSN_TEST")
	if dsn == "" {
		t.Skip("DATABASE_DSN_TEST no está definida; se omiten las pruebas con MariaDB")
	}
	if !strings.Contains(dsn, "parseTime=true") {
		t.Skip("DATABASE_DSN_TEST debe incluir parseTime=true")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("no se pudo conectar a la base de pruebas: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(esquemaPruebas); err != nil {
		t.Fatalf("no se pudo crear la tabla de pruebas: %v", err)
	}
	limpiarTabla(t, db)
	t.Cleanup(func() { limpiarTabla(t, db) })
	return db
}

func limpiarTabla(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("TRUNCATE TABLE simulaciones"); err != nil {
		t.Fatalf("no se pudo limpiar la tabla: %v", err)
	}
}

func TestMySQLCreateOrFindConcurrente(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)

	base := CreateInput{
		Usuario:       "usuario-concurrente",
		Peticion:      []byte(`{"gen":"gen4"}`),
		CacheDesde:    time.Now().Add(-time.Hour),
		MaxPendientes: 1,
		MaxCola:       100,
	}
	entradas := []CreateInput{base, base}
	entradas[0].Hash = strings.Repeat("a", 64)
	entradas[1].Hash = strings.Repeat("b", 64)

	salidas := make(chan CreateOutcome, 2)
	var wg sync.WaitGroup
	for _, in := range entradas {
		wg.Add(1)
		go func(in CreateInput) {
			defer wg.Done()
			out, err := repo.CreateOrFind(context.Background(), in)
			if err != nil {
				t.Errorf("CreateOrFind: %v", err)
				return
			}
			salidas <- out
		}(in)
	}
	wg.Wait()
	close(salidas)

	creados, limitados := 0, 0
	for out := range salidas {
		switch {
		case out.Job != nil:
			creados++
		case out.LimitUsuario:
			limitados++
		}
	}
	if creados != 1 || limitados != 1 {
		t.Fatalf("creados=%d limitados=%d", creados, limitados)
	}
}

func TestMySQLCacheYDuplicado(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	hashCache := strings.Repeat("c", 64)
	_, err := db.ExecContext(ctx, `INSERT INTO simulaciones (hash, usuario, peticion, estado, resultado, terminado)
VALUES (?, 'u-cache', '{}', 'completado', '{"gen":"gen4"}', NOW(3))`, hashCache)
	if err != nil {
		t.Fatal(err)
	}

	out, err := repo.CreateOrFind(ctx, CreateInput{
		Hash: hashCache, Usuario: "otro", Peticion: []byte(`{}`),
		CacheDesde: time.Now().Add(-time.Hour), MaxPendientes: 1, MaxCola: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Cached || out.Job == nil {
		t.Fatalf("se esperaba caché: %+v", out)
	}

	hashDup := strings.Repeat("d", 64)
	if _, err := db.ExecContext(ctx, `INSERT INTO simulaciones (hash, usuario, peticion)
VALUES (?, 'u-dup', '{}')`, hashDup); err != nil {
		t.Fatal(err)
	}
	out, err = repo.CreateOrFind(ctx, CreateInput{
		Hash: hashDup, Usuario: "u-dup", Peticion: []byte(`{}`),
		CacheDesde: time.Now().Add(-time.Hour), MaxPendientes: 1, MaxCola: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Duplicado || out.Job == nil {
		t.Fatalf("se esperaba duplicado: %+v", out)
	}
}

func TestMySQLCancelar(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	res, err := db.ExecContext(ctx, `INSERT INTO simulaciones (hash, usuario, peticion)
VALUES (?, 'dueño', '{}')`, strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if out, err := repo.Cancel(ctx, id, "otro"); err != nil || out != NoEncontrado {
		t.Fatalf("otro usuario: %v %v", out, err)
	}
	if out, err := repo.Cancel(ctx, id, "dueño"); err != nil || out != Cancelado {
		t.Fatalf("dueño: %v %v", out, err)
	}
	if out, err := repo.Cancel(ctx, id, "dueño"); err != nil || out != YaTerminado {
		t.Fatalf("segunda vez: %v %v", out, err)
	}
}

func TestMySQLGetManyYDuraciones(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	res, err := db.ExecContext(ctx, `INSERT INTO simulaciones (hash, usuario, peticion)
VALUES (?, 'u-many', '{}')`, strings.Repeat("m", 64))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	jobs, err := repo.GetMany(ctx, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Estado != EstadoPendiente {
		t.Fatalf("GetMany inesperado: %+v", jobs)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO simulaciones
(hash, usuario, peticion, estado, iniciado, terminado, resultado)
VALUES (?, 'u-many', '{}', 'completado', NOW(3) - INTERVAL 4 SECOND, NOW(3), '{}')`,
		strings.Repeat("n", 64))
	if err != nil {
		t.Fatal(err)
	}
	duraciones, err := repo.LastDurations(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(duraciones) != 1 || duraciones[0] < 3*time.Second || duraciones[0] > 6*time.Second {
		t.Fatalf("duraciones inesperadas: %v", duraciones)
	}
}

func insertarPendiente(t *testing.T, db *sql.DB, hash string, haceSegundos int) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO simulaciones (hash, usuario, peticion, creado)
VALUES (?, 'u-estados', '{"gen":"gen4"}', NOW(3) - INTERVAL ? SECOND)`, strings.Repeat(hash, 64), haceSegundos)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestMySQLEstadosDelWorker(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()
	id := insertarPendiente(t, db, "p", 0)

	comprobar := func(paso string, ok bool, err error, esperado bool) {
		t.Helper()
		if err != nil || ok != esperado {
			t.Fatalf("%s: ok=%v err=%v, se esperaba ok=%v", paso, ok, err, esperado)
		}
	}
	ok, err := repo.MarcarProcesando(ctx, id, "w1", 1)
	comprobar("procesando", ok, err, true)
	var estado, worker string
	var intentos int
	if err := db.QueryRow(`SELECT estado, worker, intentos FROM simulaciones WHERE id = ?`, id).Scan(&estado, &worker, &intentos); err != nil {
		t.Fatal(err)
	}
	if estado != EstadoProcesando || worker != "w1" || intentos != 1 {
		t.Fatalf("tras procesando: %s %s %d", estado, worker, intentos)
	}

	ok, err = repo.MarcarPendiente(ctx, id, 1)
	comprobar("pendiente", ok, err, true)
	ok, err = repo.MarcarPendiente(ctx, id, 1)
	comprobar("pendiente sin estar en proceso", ok, err, false)

	ok, err = repo.Completar(ctx, id, []byte(`{"gen":"gen4","resultados":[]}`))
	comprobar("completar", ok, err, true)
	job, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Estado != EstadoCompletado || !job.Terminado.Valid || len(job.Resultado) == 0 {
		t.Fatalf("tras completar: %+v", job)
	}

	// Un final no se pisa.
	ok, err = repo.MarcarProcesando(ctx, id, "w2", 2)
	comprobar("procesando tras completar", ok, err, false)
	ok, err = repo.Fallar(ctx, id, "tarde")
	comprobar("fallar tras completar", ok, err, false)

	cancelado := insertarPendiente(t, db, "q", 0)
	if out, err := repo.Cancel(ctx, cancelado, "u-estados"); err != nil || out != Cancelado {
		t.Fatalf("cancelar: %v %v", out, err)
	}
	ok, err = repo.Completar(ctx, cancelado, []byte(`{}`))
	comprobar("completar un cancelado", ok, err, false)

	fallido := insertarPendiente(t, db, "r", 0)
	ok, err = repo.Fallar(ctx, fallido, "Candidato 1: no existe")
	comprobar("fallar", ok, err, true)
	if job, _ := repo.Get(ctx, fallido); job.Estado != EstadoError || job.Error.String != "Candidato 1: no existe" {
		t.Fatalf("tras fallar: %+v", job)
	}
}

func TestMySQLActivosYEstados(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	viejo := insertarPendiente(t, db, "s", 60)
	medio := insertarPendiente(t, db, "u", 30)
	nuevo := insertarPendiente(t, db, "v", 0)
	if _, err := repo.MarcarProcesando(ctx, medio, "w1", 2); err != nil {
		t.Fatal(err)
	}
	terminado := insertarPendiente(t, db, "x", 90)
	if _, err := repo.Fallar(ctx, terminado, "x"); err != nil {
		t.Fatal(err)
	}

	activos, err := repo.ListActivos(ctx, time.Now().Add(-10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(activos) != 2 || activos[0].ID != viejo || activos[1].ID != medio || activos[1].Intentos != 2 {
		t.Fatalf("activos inesperados: %+v", activos)
	}
	if string(activos[0].Peticion) != `{"gen":"gen4"}` {
		t.Fatalf("petición %q", activos[0].Peticion)
	}

	estados, err := repo.EstadosDe(ctx, []int64{viejo, medio, nuevo, terminado, 999999})
	if err != nil {
		t.Fatal(err)
	}
	esperados := map[int64]string{viejo: EstadoPendiente, medio: EstadoProcesando, nuevo: EstadoPendiente, terminado: EstadoError}
	if len(estados) != len(esperados) {
		t.Fatalf("estados inesperados: %v", estados)
	}
	for id, e := range esperados {
		if estados[id] != e {
			t.Fatalf("id %d: %q, se esperaba %q", id, estados[id], e)
		}
	}
}
