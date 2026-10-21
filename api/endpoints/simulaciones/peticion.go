package simulaciones

// Stats son las estadísticas de esfuerzo o individuales de un Pokémon.
// Los campos son punteros a propósito: un IV de 0 es válido y distinto de
// "no indicado"; con omitempty un 0 se perdería y el worker asumiría 31.
type Stats struct {
	HP  *int `json:"hp,omitempty"`
	Atk *int `json:"atk,omitempty"`
	Def *int `json:"def,omitempty"`
	SpA *int `json:"spa,omitempty"`
	SpD *int `json:"spd,omitempty"`
	Spe *int `json:"spe,omitempty"`
}

type PokemonJSON struct {
	Especie     string   `json:"especie"`
	Nivel       int      `json:"nivel,omitempty"`
	Movimientos []string `json:"movimientos"`
	Habilidad   string   `json:"habilidad,omitempty"`
	Objeto      string   `json:"objeto,omitempty"`
	Naturaleza  string   `json:"naturaleza,omitempty"`
	EVs         *Stats   `json:"evs,omitempty"`
	IVs         *Stats   `json:"ivs,omitempty"`
}

type Opciones struct {
	AleatoriedadRival *float64 `json:"aleatoriedadRival,omitempty"`
	MaxTurnos         *int     `json:"maxTurnos,omitempty"`
}

// Peticion es el JSON que se guarda en la columna `peticion` y el que
// consume el worker. No cambies sus campos ni sus etiquetas.
type Peticion struct {
	Gen          string        `json:"gen"`          // "gen1" … "gen9"
	Simulaciones int           `json:"simulaciones"` // 1–2000
	Rival        PokemonJSON   `json:"rival"`
	Candidatos   []PokemonJSON `json:"candidatos"` // 1–6
	Opciones     *Opciones     `json:"opciones,omitempty"`
}

// PeticionCliente es lo que acepta el navegador. Deliberadamente no incluye
// simulaciones ni opciones: esos valores los fija el servidor.
type PeticionCliente struct {
	Gen        string        `json:"gen"`
	Rival      PokemonJSON   `json:"rival"`
	Candidatos []PokemonJSON `json:"candidatos"`
}

type ResultadoCandidato struct {
	Pokemon         string  `json:"pokemon"`
	Nivel           int     `json:"nivel"`
	Victorias       int     `json:"victorias"`
	Derrotas        int     `json:"derrotas"`
	Empates         int     `json:"empates"`
	Winrate         float64 `json:"winrate"`
	PsMedioVictoria float64 `json:"psMedioVictoria"`
	TurnosMedio     float64 `json:"turnosMedio"`
}

type Resultado struct {
	Gen          string               `json:"gen"`
	Rival        string               `json:"rival"`
	Simulaciones int                  `json:"simulaciones"`
	Resultados   []ResultadoCandidato `json:"resultados"`
	Ms           int                  `json:"ms"`
}
