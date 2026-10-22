package simulaciones

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func peticionClienteDesde(t *testing.T, cuerpo map[string]any) PeticionCliente {
	t.Helper()
	b, err := json.Marshal(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	var p PeticionCliente
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidarPeticionValida(t *testing.T) {
	p := peticionClienteDesde(t, cuerpoValido())
	if err := ValidarPeticion(&p); err != nil {
		t.Fatalf("se esperaba válida: %v", err)
	}
}

func TestValidarPeticionInvalida(t *testing.T) {
	casos := []struct {
		nombre   string
		cambiar  func(map[string]any)
		contiene string
	}{
		{
			nombre:   "gen inválida",
			cambiar:  func(c map[string]any) { c["gen"] = "gen10" },
			contiene: "gen:",
		},
		{
			nombre:   "gen en mayúsculas",
			cambiar:  func(c map[string]any) { c["gen"] = "Gen4" },
			contiene: "gen:",
		},
		{
			nombre:   "sin candidatos",
			cambiar:  func(c map[string]any) { c["candidatos"] = []any{} },
			contiene: "candidatos:",
		},
		{
			nombre: "siete candidatos",
			cambiar: func(c map[string]any) {
				base := c["candidatos"].([]any)[0]
				var lista []any
				for i := 0; i < 7; i++ {
					lista = append(lista, base)
				}
				c["candidatos"] = lista
			},
			contiene: "candidatos:",
		},
		{
			nombre: "rival sin especie",
			cambiar: func(c map[string]any) {
				c["rival"].(map[string]any)["especie"] = "   "
			},
			contiene: "rival.especie:",
		},
		{
			nombre: "rival sin movimientos",
			cambiar: func(c map[string]any) {
				c["rival"].(map[string]any)["movimientos"] = []string{}
			},
			contiene: "rival.movimientos:",
		},
		{
			nombre: "cinco movimientos",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["movimientos"] =
					[]string{"a", "b", "c", "d", "e"}
			},
			contiene: "candidatos[0].movimientos:",
		},
		{
			nombre: "movimientos repetidos",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["movimientos"] =
					[]string{"Mach Punch", "mach punch"}
			},
			contiene: "no puede haber movimientos repetidos",
		},
		{
			nombre: "texto demasiado largo",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["especie"] = strings.Repeat("a", 61)
			},
			contiene: "candidatos[0].especie:",
		},
		{
			nombre: "salto de línea",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["especie"] = "Pikachu\n- Explosion"
			},
			contiene: "caracteres de control",
		},
		{
			nombre: "nivel 101",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["nivel"] = 101
			},
			contiene: "candidatos[0].nivel:",
		},
		{
			nombre: "EV 253",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["evs"] = map[string]any{"atk": 253}
			},
			contiene: "candidatos[0].evs.atk:",
		},
		{
			nombre: "EV total 511",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["evs"] =
					map[string]any{"hp": 252, "atk": 252, "def": 7}
			},
			contiene: "suma no puede superar 510",
		},
		{
			nombre: "IV 32",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["ivs"] = map[string]any{"spe": 32}
			},
			contiene: "candidatos[0].ivs.spe:",
		},
		{
			nombre: "IV negativo",
			cambiar: func(c map[string]any) {
				c["candidatos"].([]any)[0].(map[string]any)["ivs"] = map[string]any{"spe": -1}
			},
			contiene: "candidatos[0].ivs.spe:",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cuerpo := cuerpoValido()
			caso.cambiar(cuerpo)
			p := peticionClienteDesde(t, cuerpo)
			err := ValidarPeticion(&p)
			if err == nil {
				t.Fatal("se esperaba un error de validación")
			}
			if !strings.Contains(err.Error(), caso.contiene) {
				t.Fatalf("error %q no contiene %q", err, caso.contiene)
			}
		})
	}
}

func TestLeerPeticionRechazaDesconocidos(t *testing.T) {
	cuerpo := cuerpoValido()
	cuerpo["foo"] = 1
	b, _ := json.Marshal(cuerpo)

	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !strings.Contains(err.Error(), "foo: campo desconocido") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionRechazaSimulacionesDelCliente(t *testing.T) {
	cuerpo := cuerpoValido()
	cuerpo["simulaciones"] = 2000
	b, _ := json.Marshal(cuerpo)

	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil || !strings.Contains(err.Error(), "simulaciones: campo desconocido") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionCuerpoDemasiadoGrande(t *testing.T) {
	cuerpo := `{"gen":"` + strings.Repeat("a", 70<<10) + `"}`
	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(cuerpo))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil || !strings.Contains(err.Error(), "máximo 64 KB") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionUnSoloObjeto(t *testing.T) {
	b, _ := json.Marshal(cuerpoValido())
	cuerpo := string(b) + string(b)
	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(cuerpo))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil || !strings.Contains(err.Error(), "solo se admite un objeto JSON") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionVacia(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(""))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil || !strings.Contains(err.Error(), "JSON vacío") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionTipoIncorrecto(t *testing.T) {
	cuerpo := cuerpoValido()
	cuerpo["gen"] = 4
	b, _ := json.Marshal(cuerpo)
	req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	_, err := LeerPeticion(rec, req)
	if err == nil || !strings.Contains(err.Error(), "tipo de dato inválido") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLeerPeticionErroresDeFormato(t *testing.T) {
	valido, _ := json.Marshal(cuerpoValido())
	casos := []struct {
		nombre, cuerpo, error string
	}{
		{"array en vez de objeto", `[]`, "cuerpo: tipo de dato inválido"},
		{"JSON cortado", `{"gen":`, "cuerpo: JSON inválido"},
		{"sintaxis", `{"gen" "gen4"}`, "cuerpo: JSON inválido"},
		{"basura detrás del objeto", string(valido) + ` }`, "cuerpo: JSON inválido"},
		{"objeto válido pero petición inválida", `{"gen":"gen4","rival":{},"candidatos":[]}`, "candidatos: debe haber entre 1 y 6"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/simulaciones", strings.NewReader(caso.cuerpo))
			_, err := LeerPeticion(httptest.NewRecorder(), req)
			if err == nil || err.Error() != caso.error {
				t.Fatalf("error %v, esperado %q", err, caso.error)
			}
		})
	}
}

func TestValidarPokemonCamposDeTexto(t *testing.T) {
	casos := []struct {
		campo string
		valor any
		error string
	}{
		{"habilidad", strings.Repeat("a", 200), "candidatos[0].habilidad"},
		{"objeto", "Oran\nBerry", "candidatos[0].objeto"},
		{"naturaleza", strings.Repeat("a", 200), "candidatos[0].naturaleza"},
		{"nivel", 101, "candidatos[0].nivel: debe estar entre 1 y 100"},
		{"nivel", -1, "candidatos[0].nivel: debe estar entre 1 y 100"},
	}
	for _, caso := range casos {
		t.Run(caso.campo, func(t *testing.T) {
			cuerpo := cuerpoValido()
			candidato := cuerpo["candidatos"].([]any)[0].(map[string]any)
			candidato[caso.campo] = caso.valor
			p := peticionClienteDesde(t, cuerpo)
			err := ValidarPeticion(&p)
			if err == nil || !strings.HasPrefix(err.Error(), caso.error) {
				t.Fatalf("error %v, esperado %q", err, caso.error)
			}
		})
	}
}
