package pokemons

import "PokeTeamCalculator/queries"

type PokemonPage struct {
	Page       int                           `json:"page"`
	PageSize   int                           `json:"page_size"`
	Total      int64                         `json:"total"`
	TotalPages int                           `json:"total_pages"`
	Last       bool                          `json:"last"`
	Pokemons   []queries.GetPokemonsPagerRow `json:"pokemons"`
}
