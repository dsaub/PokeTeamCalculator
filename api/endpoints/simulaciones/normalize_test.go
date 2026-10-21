package simulaciones

import "testing"

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func peticionHashBase() Peticion {
	return Peticion{
		Gen:          "gen4",
		Simulaciones: 300,
		Rival: PokemonJSON{
			Especie:     "Cranidos",
			Nivel:       14,
			Movimientos: []string{"Headbutt", "Pursuit", "Leer"},
			Habilidad:   "Mold Breaker",
		},
		Candidatos: []PokemonJSON{
			{
				Especie:     "Monferno",
				Nivel:       16,
				Naturaleza:  "Adamant",
				Movimientos: []string{"Mach Punch", "Ember"},
				EVs:         &Stats{Atk: intPtr(20)},
			},
			{
				Especie:     "Grotle",
				Nivel:       18,
				Movimientos: []string{"Razor Leaf", "Bite"},
			},
		},
		Opciones: &Opciones{AleatoriedadRival: floatPtr(0.2)},
	}
}

func TestHashEquivalente(t *testing.T) {
	base, err := HashPeticion(peticionHashBase())
	if err != nil {
		t.Fatal(err)
	}

	variante := peticionHashBase()
	variante.Rival.Especie = "cranidos"
	variante.Rival.Movimientos = []string{"head-butt", "pursuit", "LEER"}
	variante.Rival.Habilidad = "moldbreaker"
	variante.Candidatos[0].Especie = "monferno"
	variante.Candidatos[0].Naturaleza = "adamant"
	variante.Candidatos[0].Movimientos = []string{"mach-punch", "ember"}
	variante.Candidatos[1].Especie = "grotle"
	variante.Candidatos[1].Movimientos = []string{"razor-leaf", "bite"}
	// El orden de candidatos no importa.
	variante.Candidatos[0], variante.Candidatos[1] = variante.Candidatos[1], variante.Candidatos[0]

	otro, err := HashPeticion(variante)
	if err != nil {
		t.Fatal(err)
	}
	if base != otro {
		t.Fatalf("se esperaba el mismo hash:\n%s\n%s", base, otro)
	}
}

func TestHashDiferente(t *testing.T) {
	base, err := HashPeticion(peticionHashBase())
	if err != nil {
		t.Fatal(err)
	}

	casos := map[string]func(*Peticion){
		"nivel": func(p *Peticion) {
			p.Candidatos[0].Nivel = 17
			p.Candidatos[0].Nivel = 17
		},
		"iv": func(p *Peticion) {
			p.Candidatos[0].IVs = &Stats{HP: intPtr(0)}
		},
		"gen": func(p *Peticion) {
			p.Gen = "gen5"
		},
		"simulaciones": func(p *Peticion) {
			p.Simulaciones = 301
		},
		"opciones": func(p *Peticion) {
			p.Opciones.AleatoriedadRival = floatPtr(0.3)
		},
	}
	for nombre, cambiar := range casos {
		t.Run(nombre, func(t *testing.T) {
			p := peticionHashBase()
			cambiar(&p)
			otro, err := HashPeticion(p)
			if err != nil {
				t.Fatal(err)
			}
			if base == otro {
				t.Fatal("el hash debería cambiar")
			}
		})
	}
}

func TestHashIVCeroDistintoDeAusente(t *testing.T) {
	sinIV := peticionHashBase()
	conCero := peticionHashBase()
	conCero.Candidatos[0].IVs = &Stats{HP: intPtr(0)}

	a, _ := HashPeticion(sinIV)
	b, _ := HashPeticion(conCero)
	if a == b {
		t.Fatal("un IV de 0 no puede equivaler a no indicar IV")
	}
}

func TestHashNivelPorDefecto(t *testing.T) {
	sinNivel := peticionHashBase()
	sinNivel.Candidatos[1].Nivel = 0
	conCien := peticionHashBase()
	conCien.Candidatos[1].Nivel = 100

	a, _ := HashPeticion(sinNivel)
	b, _ := HashPeticion(conCien)
	if a != b {
		t.Fatal("el nivel ausente debe normalizarse a 100")
	}
}

func TestHashNidoranNoColisiona(t *testing.T) {
	hembra := peticionHashBase()
	hembra.Rival.Especie = "Nidoran♀"
	macho := peticionHashBase()
	macho.Rival.Especie = "Nidoran♂"

	a, _ := HashPeticion(hembra)
	b, _ := HashPeticion(macho)
	if a == b {
		t.Fatal("Nidoran♀ y Nidoran♂ no pueden compartir hash")
	}
}

func TestToID(t *testing.T) {
	casos := map[string]string{
		"Mach Punch": "machpunch",
		"mach-punch": "machpunch",
		"Mr. Mime":   "mrmime",
		"Farfetch'd": "farfetchd",
		"  Leer  ":   "leer",
	}
	for entrada, esperado := range casos {
		if got := toID(entrada); got != esperado {
			t.Errorf("toID(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}
