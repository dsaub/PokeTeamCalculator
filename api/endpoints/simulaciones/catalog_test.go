package simulaciones

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var columnasHabilidadesCatalogo = []string{"slot", "hidden", "ability_id", "gen", "ident", "pasada"}

func catalogoFalsoSQL(t *testing.T) (*MySQLCatalog, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return NewMySQLCatalog(db), mock
}

func TestCatalogoHabilidadValidaConSQLFalso(t *testing.T) {
	cat, mock := catalogoFalsoSQL(t)
	mock.ExpectQuery(`SELECT id FROM pokemon`).WithArgs("cranidos").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(408))
	mock.ExpectQuery(`pokemon_abilities pa`).WithArgs(408, 408).WillReturnRows(sqlmock.NewRows(columnasHabilidadesCatalogo).
		AddRow(1, false, 104, 4, "mold-breaker", 0).
		AddRow(3, true, 125, 5, "sheer-force", 0))

	existe, valida, err := cat.HabilidadValida(context.Background(), 4, "Cranidos", "Mold Breaker")
	if err != nil || !existe || !valida {
		t.Fatalf("existe=%v valida=%v err=%v", existe, valida, err)
	}
}

func TestCatalogoHabilidadValidaCasos(t *testing.T) {
	errBase := errors.New("caída")
	casos := []struct {
		nombre         string
		especie        string
		preparar       func(sqlmock.Sqlmock)
		existe, valida bool
		conError       bool
	}{
		{"especie vacía: no consulta", " -- ", func(sqlmock.Sqlmock) {}, false, false, false},
		{"especie inexistente", "Missingno", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		}, false, false, false},
		{"error al buscar la especie", "Cranidos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnError(errBase)
		}, false, false, true},
		{"error al leer habilidades", "Cranidos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectQuery(`pokemon_abilities`).WillReturnError(errBase)
		}, true, false, true},
		{"fila con otro tipo", "Cranidos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidadesCatalogo).AddRow("x", false, 1, 3, "a", 0))
		}, true, false, true},
		{"error al recorrer", "Cranidos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidadesCatalogo).AddRow(1, false, 1, 3, "a", 0).RowError(0, errBase))
		}, true, false, true},
		{"habilidad que no tiene", "Cranidos", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`SELECT id FROM pokemon`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectQuery(`pokemon_abilities`).WillReturnRows(sqlmock.NewRows(columnasHabilidadesCatalogo).AddRow(1, false, 104, 4, "mold-breaker", 0))
		}, true, false, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cat, mock := catalogoFalsoSQL(t)
			caso.preparar(mock)
			existe, valida, err := cat.HabilidadValida(context.Background(), 4, caso.especie, "Levitate")
			if existe != caso.existe || valida != caso.valida || (err != nil) != caso.conError {
				t.Fatalf("existe=%v valida=%v err=%v", existe, valida, err)
			}
		})
	}
}
