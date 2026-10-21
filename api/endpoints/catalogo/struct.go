package catalogo

// Juego es una entrega con gimnasios. VersionGroup es interno: se usa para
// consultar los líderes y no se expone en el JSON.
type Juego struct {
	ID           string `json:"id"`
	Nombre       string `json:"nombre"`
	Gen          string `json:"gen"`
	VersionGroup string `json:"-"`
}

type Lider struct {
	ID     int64           `json:"id"`
	Nombre string          `json:"nombre"`
	Ciudad string          `json:"ciudad"`
	Tipo   *string         `json:"tipo"`
	Orden  int             `json:"orden"`
	Equipo []PokemonEquipo `json:"equipo"`
}

type PokemonEquipo struct {
	Especie     string   `json:"especie"`
	Nombre      string   `json:"nombre"`
	Nivel       int      `json:"nivel"`
	Habilidad   *string  `json:"habilidad"`
	Objeto      *string  `json:"objeto"`
	Movimientos []string `json:"movimientos"`
	Tipos       []string `json:"tipos"`
}

type PokemonResumen struct {
	Especie string   `json:"especie"`
	Nombre  string   `json:"nombre"`
	Tipos   []string `json:"tipos"`
}

type Habilidad struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

type Movimiento struct {
	ID        string  `json:"id"`
	Nombre    string  `json:"nombre"`
	Tipo      string  `json:"tipo"`
	Categoria *string `json:"categoria"`
	Potencia  *int    `json:"potencia"`
}

type PokemonDetalle struct {
	Especie     string       `json:"especie"`
	Nombre      string       `json:"nombre"`
	Tipos       []string     `json:"tipos"`
	Habilidades []Habilidad  `json:"habilidades"`
	Movimientos []Movimiento `json:"movimientos"`
}
