package structs

type Status struct {
	Status string `json:"status"`
}

type Gym struct {
	GymBattleID   int    `json:"gym_battle_id"`
	Gimnasio      string `json:"gimnasio"`
	Lider         string `json:"lider"`
	Medalla       string `json:"medalla"`
	Tipo          string `json:"tipo"`
	Juego         string `json:"juego"`
	VersionGroup  string `json:"version_group"`
	OrdenJuego    string `json:"orden_juego"`
	NumeroMedalla string `json:"numero_medalla"`
	TipoCombate   string `json:"tipo_combate"`
}

type Gyms struct {
	Gimnasios []Gym `json:"gimnasios"`
}
