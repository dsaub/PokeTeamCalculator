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
	if !out.Duplicado || out.Job == nil || out.Posicion != 1 {
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
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Estado != EstadoPendiente || jobs[0].Posicion != 1 {
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

func TestMySQLPosiciones(t *testing.T) {
	db := abrirBDPruebas(t)
	repo := NewMySQLRepository(db)
	ctx := context.Background()

	var ids []int64
	for i := 0; i < 3; i++ {
		res, err := db.ExecContext(ctx, `INSERT INTO simulaciones (hash, usuario, peticion, creado)
VALUES (?, 'u-pos', '{}', NOW(3) - INTERVAL ? SECOND)`, strings.Repeat(string(rune('f'+i)), 64), 30-i)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	posiciones, err := repo.PendingPositions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		if posiciones[id] != i+1 {
			t.Fatalf("id %d: posición %d, se esperaba %d", id, posiciones[id], i+1)
		}
	}
}
