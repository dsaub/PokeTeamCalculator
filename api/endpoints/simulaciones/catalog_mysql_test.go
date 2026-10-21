package simulaciones

import (
	"context"
	"testing"
)

func TestMySQLCatalogoHabilidadValida(t *testing.T) {
	db := abrirBDPruebas(t)
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = 'pokemon'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Skip("la base de DATABASE_DSN_TEST no tiene pokedata (falta pokemon)")
	}

	cat := NewMySQLCatalog(db)
	ctx := context.Background()

	existe, valida, err := cat.HabilidadValida(ctx, 4, "Cranidos", "Mold Breaker")
	if err != nil || !existe || !valida {
		t.Fatalf("Cranidos/Mold Breaker: existe=%v valida=%v err=%v", existe, valida, err)
	}
	existe, valida, err = cat.HabilidadValida(ctx, 4, "Cranidos", "Levitate")
	if err != nil || !existe || valida {
		t.Fatalf("Cranidos/Levitate: existe=%v valida=%v err=%v", existe, valida, err)
	}
	existe, _, err = cat.HabilidadValida(ctx, 4, "NoExiste", "Mold Breaker")
	if err != nil || existe {
		t.Fatalf("especie inexistente: existe=%v err=%v", existe, err)
	}
}
