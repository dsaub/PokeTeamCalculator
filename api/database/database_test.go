package database

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("PRUEBA_DB", "")
	if v := getEnv("PRUEBA_DB", "defecto"); v != "defecto" {
		t.Fatalf("vacía: %q", v)
	}
	t.Setenv("PRUEBA_DB", "valor")
	if v := getEnv("PRUEBA_DB", "defecto"); v != "valor" {
		t.Fatalf("definida: %q", v)
	}
}

func TestLoadConfig(t *testing.T) {
	for _, k := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_TLS"} {
		t.Setenv(k, "")
	}
	got := loadConfig()
	want := config{host: "127.0.0.1", port: "3306", user: "root", password: "", name: "pokedata", tls: "preferred"}
	if got != want {
		t.Fatalf("por defecto %+v, esperado %+v", got, want)
	}

	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USER", "poke")
	t.Setenv("DB_PASSWORD", "secreta")
	t.Setenv("DB_NAME", "otra")
	t.Setenv("DB_TLS", "true")
	got = loadConfig()
	want = config{host: "db", port: "3307", user: "poke", password: "secreta", name: "otra", tls: "true"}
	if got != want {
		t.Fatalf("del entorno %+v, esperado %+v", got, want)
	}
}

func TestEstado(t *testing.T) {
	anterior := GetStatus()
	t.Cleanup(func() { SetStatus(anterior) })

	SetStatus("ok")
	if s := GetStatus(); s != "ok" {
		t.Fatalf("estado %q", s)
	}

	// Lecturas y escrituras concurrentes (go test -race).
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); SetStatus("error") }()
		go func() { defer wg.Done(); _ = GetStatus() }()
	}
	wg.Wait()
	if s := GetStatus(); s != "error" {
		t.Fatalf("estado %q", s)
	}
}

func TestGetDatabaseDevuelveLaGlobal(t *testing.T) {
	anterior := DB
	t.Cleanup(func() { DB = anterior })
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	DB = db
	if GetDatabase() != db {
		t.Fatal("GetDatabase no devuelve DB")
	}
}

func TestWaitForDatabase(t *testing.T) {
	t.Run("responde a la primera", func(t *testing.T) {
		db, mock := conPings(t)
		mock.ExpectPing()
		if err := waitForDatabase(db, 3, time.Millisecond); err != nil {
			t.Fatal(err)
		}
		comprobar(t, mock)
	})

	t.Run("reintenta hasta que responde", func(t *testing.T) {
		db, mock := conPings(t)
		mock.ExpectPing().WillReturnError(errors.New("arrancando"))
		mock.ExpectPing().WillReturnError(errors.New("arrancando"))
		mock.ExpectPing()
		if err := waitForDatabase(db, 5, time.Millisecond); err != nil {
			t.Fatal(err)
		}
		comprobar(t, mock)
	})

	t.Run("se rinde y devuelve el último error", func(t *testing.T) {
		db, mock := conPings(t)
		mock.ExpectPing().WillReturnError(errors.New("uno"))
		mock.ExpectPing().WillReturnError(errors.New("dos"))
		err := waitForDatabase(db, 2, time.Millisecond)
		if err == nil || err.Error() != "dos" {
			t.Fatalf("error %v, esperado «dos»", err)
		}
		comprobar(t, mock)
	})
}

func TestIsPopulated(t *testing.T) {
	casos := []struct {
		nombre string
		filas  int
		err    error
		lleno  bool
	}{
		{"con la vista", 1, nil, true},
		{"sin la vista", 0, nil, false},
		{"error de la base", 0, errors.New("sin permiso"), false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			q := mock.ExpectQuery(`information_schema\.views`).WithArgs("pokedata")
			if caso.err != nil {
				q.WillReturnError(caso.err)
			} else {
				q.WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(caso.filas))
			}
			lleno, err := isPopulated(db, "pokedata")
			if lleno != caso.lleno || !errors.Is(err, caso.err) {
				t.Fatalf("(%v, %v), esperado (%v, %v)", lleno, err, caso.lleno, caso.err)
			}
			comprobar(t, mock)
		})
	}
}

func conPings(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func comprobar(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
