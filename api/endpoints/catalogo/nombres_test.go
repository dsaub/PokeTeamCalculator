package catalogo

import (
	"testing"
)

func TestSlug(t *testing.T) {
	casos := map[string]string{
		"Platino":            "platino",
		"Diamante y Perla":   "diamante-y-perla",
		"Rubí Omega":         "rubi-omega",
		"Let’s Go, Pikachu!": "lets-go-pikachu",
		"Pokémon X":          "pokemon-x",
		"  Oro   HeartGold ": "oro-heartgold",
	}
	for entrada, esperado := range casos {
		if got := Slug(entrada); got != esperado {
			t.Errorf("Slug(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

func TestNombreJuego(t *testing.T) {
	if got := NombreJuego([]string{"Platino"}); got != "Pokémon Platino" {
		t.Fatalf("NombreJuego: %q", got)
	}
	if got := NombreJuego([]string{"Diamante", "Perla"}); got != "Pokémon Diamante y Perla" {
		t.Fatalf("NombreJuego: %q", got)
	}
}

func TestNombreShowdown(t *testing.T) {
	casos := []struct {
		identifier, species string
		isDefault           bool
		base, esperado      string
	}{
		{"yamask-galar", "yamask", false, "Yamask", "Yamask-Galar"},
		{"weezing-galar", "weezing", false, "Weezing", "Weezing-Galar"},
		{"darmanitan-galar-standard", "darmanitan", false, "Darmanitan", "Darmanitan-Galar"},
		{"darmanitan-galar-zen", "darmanitan", false, "Darmanitan", "Darmanitan-Galar-Zen"},
		{"toxtricity-low-key", "toxtricity", false, "Toxtricity", "Toxtricity-Low-Key"},
		{"meowstic-female", "meowstic", false, "Meowstic", "Meowstic-Female"},
		{"rotom-wash", "rotom", false, "Rotom", "Rotom-Wash"},
		{"deoxys-attack", "deoxys", false, "Deoxys", "Deoxys-Attack"},
		{"deoxys-normal", "deoxys", true, "Deoxys", "Deoxys"},
		{"pikachu", "pikachu", true, "Pikachu", "Pikachu"},
		{"mr-mime-galar", "mr-mime", false, "Mr. Mime", "Mr. Mime-Galar"},
	}
	for _, c := range casos {
		if got := NombreShowdown(c.identifier, c.species, c.isDefault, c.base); got != c.esperado {
			t.Errorf("NombreShowdown(%q) = %q, se esperaba %q", c.identifier, got, c.esperado)
		}
	}
}

func TestNormalizarEspecie(t *testing.T) {
	casos := map[string]string{
		"Nidoran♀":      "nidoranf",
		"Nidoran♂":      "nidoranm",
		"Farfetch’d":    "farfetchd",
		"Mr. Mime":      "mrmime",
		"Type: Null":    "typenull",
		"Ho-Oh":         "hooh",
		"Weezing-Galar": "weezinggalar",
	}
	for entrada, esperado := range casos {
		if got := normalizarEspecie(entrada); got != esperado {
			t.Errorf("normalizarEspecie(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

func TestSepararLista(t *testing.T) {
	got := separarLista("Tackle, Growl, Ember")
	if len(got) != 3 || got[0] != "Tackle" || got[2] != "Ember" {
		t.Fatalf("separarLista: %#v", got)
	}
	if len(separarLista("")) != 0 {
		t.Fatal("una lista vacía debe devolver []")
	}
}
