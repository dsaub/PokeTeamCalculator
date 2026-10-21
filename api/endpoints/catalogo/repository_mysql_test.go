package catalogo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func abrirPokedata(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_DSN_TEST")
	if dsn == "" {
		t.Skip("DATABASE_DSN_TEST no está definida; se omiten las pruebas de catálogo con MariaDB")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("no se pudo conectar: %v", err)
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.views
WHERE table_schema = DATABASE() AND table_name = 'v_gym_teams'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Skip("la base de DATABASE_DSN_TEST no tiene pokedata (falta v_gym_teams)")
	}
	return db
}

func TestMySQLCatalogoJuegos(t *testing.T) {
	repo := NewMySQLRepository(abrirPokedata(t))
	juegos, err := repo.Juegos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(juegos) == 0 {
		t.Fatal("no se listó ningún juego")
	}
	var platino *Juego
	for i := range juegos {
		if juegos[i].ID == "platino" {
			platino = &juegos[i]
			break
		}
	}
	if platino == nil {
		t.Fatalf("no se encontró el juego platino: %+v", juegos)
	}
	if platino.Gen != "gen4" || platino.Nombre != "Pokémon Platino" || platino.VersionGroup != "platinum" {
		t.Fatalf("platino inesperado: %+v", platino)
	}
}

func TestMySQLCatalogoLideres(t *testing.T) {
	repo := NewMySQLRepository(abrirPokedata(t))
	lideres, err := repo.Lideres(context.Background(), "platinum", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(lideres) != 8 {
		t.Fatalf("se esperaban 8 líderes, hay %d", len(lideres))
	}
	if lideres[0].Nombre != "Roco" || lideres[0].Orden != 1 {
		t.Fatalf("primer líder: %+v", lideres[0])
	}
	if len(lideres[0].Equipo) == 0 {
		t.Fatal("el primer líder no tiene equipo")
	}
	for _, l := range lideres {
		for _, pk := range l.Equipo {
			if pk.Especie == "" || pk.Nombre == "" || len(pk.Tipos) == 0 {
				t.Fatalf("Pokémon incompleto: %+v", pk)
			}
		}
	}
}

func TestMySQLCatalogoFormas(t *testing.T) {
	repo := NewMySQLRepository(abrirPokedata(t))
	lideres, err := repo.Lideres(context.Background(), "sword-shield", 8)
	if err != nil {
		t.Fatal(err)
	}
	var vistos = map[string]bool{}
	for _, l := range lideres {
		for _, pk := range l.Equipo {
			vistos[pk.Especie] = true
		}
	}
	for _, esperado := range []string{"Yamask-Galar", "Weezing-Galar", "Darmanitan-Galar"} {
		if !vistos[esperado] {
			t.Errorf("falta %s en los equipos de sword-shield", esperado)
		}
	}
}

func TestMySQLCatalogoBuscar(t *testing.T) {
	repo := NewMySQLRepository(abrirPokedata(t))
	ctx := context.Background()

	res, err := repo.BuscarPokemon(ctx, 4, "monf", 20)
	if err != nil {
		t.Fatal(err)
	}
	var monferno bool
	for _, r := range res {
		if r.Especie == "Monferno" {
			monferno = true
		}
	}
	if !monferno {
		t.Fatalf("no se encontró Monferno: %+v", res)
	}

	res, err = repo.BuscarPokemon(ctx, 4, "weez", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Especie == "Weezing-Galar" {
			t.Fatal("Weezing-Galar no existe en gen 4")
		}
	}

	res, err = repo.BuscarPokemon(ctx, 8, "weez", 20)
	if err != nil {
		t.Fatal(err)
	}
	var galar bool
	for _, r := range res {
		if r.Especie == "Weezing-Galar" {
			galar = true
		}
	}
	if !galar {
		t.Fatalf("no se encontró Weezing-Galar en gen 8: %+v", res)
	}

	res, err = repo.BuscarPokemon(ctx, 8, "galarian", 20)
	if err != nil {
		t.Fatal(err)
	}
	galar = false
	for _, r := range res {
		if r.Especie == "Weezing-Galar" {
			galar = true
		}
	}
	if !galar {
		t.Fatalf("no se encontró Weezing-Galar buscando por nombre inglés: %+v", res)
	}
}

func TestMySQLCatalogoDetalle(t *testing.T) {
	repo := NewMySQLRepository(abrirPokedata(t))
	ctx := context.Background()

	detalle, err := repo.Pokemon(ctx, 8, "Weezing-Galar")
	if err != nil {
		t.Fatal(err)
	}
	if detalle.Especie != "Weezing-Galar" || detalle.Nombre == "" {
		t.Fatalf("detalle: %+v", detalle)
	}
	if len(detalle.Tipos) != 2 || len(detalle.Habilidades) == 0 || len(detalle.Movimientos) == 0 {
		t.Fatalf("detalle incompleto: %+v", detalle)
	}

	if _, err := repo.Pokemon(ctx, 4, "Weezing-Galar"); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("Weezing-Galar en gen 4 debería dar 404: %v", err)
	}

	pikachu, err := repo.Pokemon(ctx, 1, "Pikachu")
	if err != nil {
		t.Fatal(err)
	}
	if len(pikachu.Habilidades) != 0 {
		t.Fatalf("gen 1 no debe tener habilidades: %+v", pikachu.Habilidades)
	}
	if len(pikachu.Movimientos) == 0 {
		t.Fatal("Pikachu debería tener movimientos en gen 1")
	}

	nidoran, err := repo.Pokemon(ctx, 1, "Nidoran♀")
	if err != nil {
		t.Fatal(err)
	}
	if nidoran.Especie != "Nidoran♀" {
		t.Fatalf("Nidoran♀: %q", nidoran.Especie)
	}
}
