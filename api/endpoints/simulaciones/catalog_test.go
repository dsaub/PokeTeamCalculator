package simulaciones

import "testing"

func TestHabilidadEnFilas(t *testing.T) {
	filas := []filaHabilidad{
		// Actuales.
		{slot: 1, abilityID: 10, generacion: 4, ident: "mold-breaker"},
		{slot: 2, abilityID: 11, generacion: 5, ident: "sheer-force"},
		// Override para gen 4: el slot 2 no existía.
		{slot: 2, abilityID: 0, generacion: 0, ident: "", pasadaGen: 4},
		// Habilidad oculta actual.
		{slot: 3, hidden: true, abilityID: 12, generacion: 3, ident: "sand-force"},
	}

	if !habilidadEnFilas(filas, 4, "Mold Breaker") {
		t.Fatal("Mold Breaker debería ser válida en gen 4")
	}
	if !habilidadEnFilas(filas, 4, "moldbreaker") {
		t.Fatal("la normalización toID debería aplicar")
	}
	if habilidadEnFilas(filas, 4, "Sheer Force") {
		t.Fatal("Sheer Force no existía en gen 4")
	}
	if !habilidadEnFilas(filas, 5, "Sheer Force") {
		t.Fatal("Sheer Force debería ser válida en gen 5")
	}
	if !habilidadEnFilas(filas, 5, "Sand Force") {
		t.Fatal("las habilidades ocultas deberían contar")
	}
	if habilidadEnFilas(filas, 4, "No Existe") {
		t.Fatal("una habilidad inexistente no puede ser válida")
	}
}
