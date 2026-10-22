package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var cfgPrueba = config{host: "db", port: "3306", user: "poke", password: "secreta", name: "pokedata", tls: "preferred"}

// importadorPrueba devuelve un importador con una migración de prueba y que
// anota las llamadas al binario en vez de ejecutarlo.
func importadorPrueba(t *testing.T, errEjecutar error) (*importador, *[][]string) {
	t.Helper()
	migracion := filepath.Join(t.TempDir(), "003.sql")
	if err := os.WriteFile(migracion, []byte("CREATE OR REPLACE VIEW v AS SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	llamadas := &[][]string{}
	return &importador{
		binario:   "/pokedata-gen",
		migracion: migracion,
		ejecutar: func(_ context.Context, binario string, args ...string) error {
			*llamadas = append(*llamadas, append([]string{binario}, args...))
			return errEjecutar
		},
	}, llamadas
}

func esperarCerrojo(mock sqlmock.Sqlmock, obtenido int) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT GET_LOCK(?, ?)")).
		WithArgs(cerrojoPokedata, esperaCerrojoSeg).
		WillReturnRows(sqlmock.NewRows([]string{"l"}).AddRow(obtenido))
}

func esperarVista(mock sqlmock.Sqlmock, filas int) {
	mock.ExpectQuery(`information_schema\.views`).WithArgs("pokedata").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(filas))
}

func esperarLiberar(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("SELECT RELEASE_LOCK(?)")).
		WithArgs(cerrojoPokedata).WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestImportarCargaYAplicaLaMigracion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	im, llamadas := importadorPrueba(t, nil)

	esperarCerrojo(mock, 1)
	esperarVista(mock, 0)
	mock.ExpectExec(regexp.QuoteMeta("CREATE OR REPLACE VIEW v AS SELECT 1;")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	esperarLiberar(mock)

	if err := im.importar(context.Background(), db, cfgPrueba); err != nil {
		t.Fatal(err)
	}
	if len(*llamadas) != 1 || (*llamadas)[0][0] != "/pokedata-gen" {
		t.Fatalf("llamadas %v", *llamadas)
	}
	comprobar(t, mock)
}

func TestImportarNoRepiteSiOtraInstanciaYaImporto(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	im, llamadas := importadorPrueba(t, nil)

	esperarCerrojo(mock, 1)
	esperarVista(mock, 1)
	esperarLiberar(mock)

	if err := im.importar(context.Background(), db, cfgPrueba); err != nil {
		t.Fatal(err)
	}
	if len(*llamadas) != 0 {
		t.Fatalf("ejecutó pokedata-gen: %v", *llamadas)
	}
	comprobar(t, mock)
}

func TestImportarSinCerrojo(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	im, llamadas := importadorPrueba(t, nil)

	esperarCerrojo(mock, 0)

	if err := im.importar(context.Background(), db, cfgPrueba); err == nil {
		t.Fatal("esperaba error")
	}
	if len(*llamadas) != 0 {
		t.Fatalf("ejecutó pokedata-gen: %v", *llamadas)
	}
	comprobar(t, mock)
}

func TestImportarFallaPokedataNoAplicaLaMigracion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	fallo := errors.New("sin red")
	im, _ := importadorPrueba(t, fallo)

	esperarCerrojo(mock, 1)
	esperarVista(mock, 0)
	esperarLiberar(mock)

	if err := im.importar(context.Background(), db, cfgPrueba); !errors.Is(err, fallo) {
		t.Fatalf("error %v, esperado %v", err, fallo)
	}
	comprobar(t, mock)
}

func TestArgumentosPokedata(t *testing.T) {
	got := argumentosPokedata(cfgPrueba, "/tmp/fuentes")
	want := []string{
		"--mode", "mysql", "--host", "db", "--port", "3306", "--username", "poke",
		"--password", "secreta", "--database", "pokedata", "--tls", "preferred",
		"--sources", "/tmp/fuentes",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%v, esperado %v", got, want)
	}
}

func TestImportadorDisponible(t *testing.T) {
	binario := filepath.Join(t.TempDir(), "pokedata-gen")
	t.Setenv("POKEDATA_GEN", binario)
	if nuevoImportador().disponible() {
		t.Fatal("disponible sin binario")
	}
	if err := os.WriteFile(binario, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if !nuevoImportador().disponible() {
		t.Fatal("no disponible con binario")
	}
}
