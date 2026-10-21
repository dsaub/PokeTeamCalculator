package simulaciones

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxCuerpoBytes = 64 << 10
	maxTexto       = 60
	maxCandidatos  = 6
	maxMovimientos = 4
	maxEVStat      = 252
	maxEVTotal     = 510
	maxIV          = 31
)

var genValida = regexp.MustCompile(`^gen[1-9]$`)

// ValidationError indica qué campo del cuerpo no es válido.
type ValidationError struct {
	Campo   string
	Mensaje string
}

func (e *ValidationError) Error() string {
	if e.Campo == "" {
		return e.Mensaje
	}
	return e.Campo + ": " + e.Mensaje
}

func errorCampo(campo, mensaje string) error {
	return &ValidationError{Campo: campo, Mensaje: mensaje}
}

// LeerPeticion limita el cuerpo a 64 KB, rechaza campos desconocidos y valida
// la forma de la petición. No comprueba la Pokédex: eso es cosa del worker.
func LeerPeticion(w http.ResponseWriter, r *http.Request) (*PeticionCliente, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var p PeticionCliente
	if err := dec.Decode(&p); err != nil {
		return nil, traducirErrorJSON(err)
	}

	// Debe haber un único objeto JSON: rechazamos contenido extra.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errorCampo("cuerpo", "solo se admite un objeto JSON")
		}
		return nil, traducirErrorJSON(err)
	}

	if err := ValidarPeticion(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func traducirErrorJSON(err error) error {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return errorCampo("cuerpo", "máximo 64 KB")
	}

	var tipo *json.UnmarshalTypeError
	if errors.As(err, &tipo) {
		campo := tipo.Field
		if campo == "" {
			campo = "cuerpo"
		}
		return errorCampo(campo, "tipo de dato inválido")
	}

	var sintaxis *json.SyntaxError
	if errors.As(err, &sintaxis) {
		return errorCampo("cuerpo", "JSON inválido")
	}

	if errors.Is(err, io.EOF) {
		return errorCampo("cuerpo", "JSON vacío")
	}

	const prefijo = `json: unknown field "`
	if msg := err.Error(); strings.HasPrefix(msg, prefijo) {
		campo := strings.TrimSuffix(strings.TrimPrefix(msg, prefijo), `"`)
		return errorCampo(campo, "campo desconocido")
	}

	return errorCampo("cuerpo", "JSON inválido")
}

// ValidarPeticion valida gen, rival y candidatos.
func ValidarPeticion(p *PeticionCliente) error {
	if !genValida.MatchString(p.Gen) {
		return errorCampo("gen", "debe ser gen1, gen2, …, gen9")
	}
	if len(p.Candidatos) < 1 || len(p.Candidatos) > maxCandidatos {
		return errorCampo("candidatos", "debe haber entre 1 y 6")
	}

	if err := validarPokemon("rival", &p.Rival); err != nil {
		return err
	}
	for i := range p.Candidatos {
		if err := validarPokemon(fmt.Sprintf("candidatos[%d]", i), &p.Candidatos[i]); err != nil {
			return err
		}
	}
	return nil
}

func validarPokemon(campo string, p *PokemonJSON) error {
	if err := validarTexto(campo+".especie", p.Especie, true); err != nil {
		return err
	}
	if len(p.Movimientos) < 1 || len(p.Movimientos) > maxMovimientos {
		return errorCampo(campo+".movimientos", "debe haber entre 1 y 4")
	}
	vistos := make(map[string]struct{}, len(p.Movimientos))
	for i, mov := range p.Movimientos {
		if err := validarTexto(fmt.Sprintf("%s.movimientos[%d]", campo, i), mov, true); err != nil {
			return err
		}
		id := toID(mov)
		if _, repetido := vistos[id]; repetido {
			return errorCampo(campo+".movimientos", "no puede haber movimientos repetidos")
		}
		vistos[id] = struct{}{}
	}
	if err := validarTexto(campo+".habilidad", p.Habilidad, false); err != nil {
		return err
	}
	if err := validarTexto(campo+".objeto", p.Objeto, false); err != nil {
		return err
	}
	if err := validarTexto(campo+".naturaleza", p.Naturaleza, false); err != nil {
		return err
	}

	if p.Nivel != 0 && (p.Nivel < 1 || p.Nivel > 100) {
		return errorCampo(campo+".nivel", "debe estar entre 1 y 100")
	}

	if err := validarStats(campo+".evs", p.EVs, maxEVStat, true); err != nil {
		return err
	}
	if p.EVs != nil && totalStats(p.EVs) > maxEVTotal {
		return errorCampo(campo+".evs", "la suma no puede superar 510")
	}
	if err := validarStats(campo+".ivs", p.IVs, maxIV, false); err != nil {
		return err
	}
	return nil
}

func validarTexto(campo, valor string, obligatorio bool) error {
	if obligatorio && strings.TrimSpace(valor) == "" {
		return errorCampo(campo, "no puede estar vacío")
	}
	if valor == "" {
		return nil
	}
	if strings.IndexFunc(valor, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return errorCampo(campo, "no puede contener caracteres de control")
	}
	if utf8.RuneCountInString(valor) > maxTexto {
		return errorCampo(campo, "máximo 60 caracteres")
	}
	return nil
}

func validarStats(campo string, s *Stats, maximo int, evs bool) error {
	if s == nil {
		return nil
	}
	valores := []struct {
		nombre string
		valor  *int
	}{
		{"hp", s.HP}, {"atk", s.Atk}, {"def", s.Def},
		{"spa", s.SpA}, {"spd", s.SpD}, {"spe", s.Spe},
	}
	for _, v := range valores {
		if v.valor == nil {
			continue
		}
		if *v.valor < 0 || *v.valor > maximo {
			if evs {
				return errorCampo(campo+"."+v.nombre, "debe estar entre 0 y 252")
			}
			return errorCampo(campo+"."+v.nombre, "debe estar entre 0 y 31")
		}
	}
	return nil
}

func totalStats(s *Stats) int {
	total := 0
	for _, v := range []*int{s.HP, s.Atk, s.Def, s.SpA, s.SpD, s.Spe} {
		if v != nil {
			total += *v
		}
	}
	return total
}
