package simulaciones

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

var simbolosGenero = strings.NewReplacer("♀", "f", "♂", "m", "×", "x")

// toID imita el toID de Pokémon Showdown: minúsculas y solo [a-z0-9].
// Los símbolos de género se conservan como f/m para que Nidoran♀ y Nidoran♂
// no acaben con el mismo hash.
func toID(s string) string {
	s = strings.ToLower(simbolosGenero.Replace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type pokemonNormalizado struct {
	Especie     string   `json:"especie"`
	Nivel       int      `json:"nivel"`
	Movimientos []string `json:"movimientos"`
	Habilidad   string   `json:"habilidad,omitempty"`
	Objeto      string   `json:"objeto,omitempty"`
	Naturaleza  string   `json:"naturaleza,omitempty"`
	EVs         *Stats   `json:"evs,omitempty"`
	IVs         *Stats   `json:"ivs,omitempty"`
}

type peticionNormalizada struct {
	Gen          string               `json:"gen"`
	Simulaciones int                  `json:"simulaciones"`
	Rival        pokemonNormalizado   `json:"rival"`
	Candidatos   []pokemonNormalizado `json:"candidatos"`
	Opciones     *Opciones            `json:"opciones,omitempty"`
}

func normalizarPokemon(p PokemonJSON) pokemonNormalizado {
	nivel := p.Nivel
	if nivel == 0 {
		nivel = 100
	}
	movs := make([]string, len(p.Movimientos))
	for i, m := range p.Movimientos {
		movs[i] = toID(m)
	}
	return pokemonNormalizado{
		Especie:     toID(p.Especie),
		Nivel:       nivel,
		Movimientos: movs,
		Habilidad:   toID(p.Habilidad),
		Objeto:      toID(p.Objeto),
		Naturaleza:  toID(p.Naturaleza),
		EVs:         p.EVs,
		IVs:         p.IVs,
	}
}

// NormalizarPeticion devuelve una copia canónica: nombres toID, nivel 100 si
// falta y candidatos ordenados por su JSON normalizado.
func NormalizarPeticion(p Peticion) peticionNormalizada {
	candidatos := make([]pokemonNormalizado, len(p.Candidatos))
	for i, c := range p.Candidatos {
		candidatos[i] = normalizarPokemon(c)
	}
	sort.SliceStable(candidatos, func(i, j int) bool {
		return jsonMenor(candidatos[i], candidatos[j])
	})
	return peticionNormalizada{
		Gen:          p.Gen,
		Simulaciones: p.Simulaciones,
		Rival:        normalizarPokemon(p.Rival),
		Candidatos:   candidatos,
		Opciones:     p.Opciones,
	}
}

func jsonMenor(a, b pokemonNormalizado) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Compare(ja, jb) < 0
}

// HashPeticion calcula sha256 del JSON normalizado.
func HashPeticion(p Peticion) (string, error) {
	b, err := json.Marshal(NormalizarPeticion(p))
	if err != nil {
		return "", err
	}
	suma := sha256.Sum256(b)
	return hex.EncodeToString(suma[:]), nil
}
