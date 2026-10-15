package pokemons

import "PokeTeamCalculator/queries"

type PokemonPage struct {
	Page     int                           `json:"page"`
	Pokemons []queries.GetPokemonsPagerRow `json:"pokemons"`
	Last     bool                          `json:"last"`
	Elements int                           `json:"elements"`
}
