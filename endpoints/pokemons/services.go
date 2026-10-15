package pokemons

import (
	"PokeTeamCalculator/database"
	"PokeTeamCalculator/queries"
	"context"
)

func ServiceGetPokemons(page int) (PokemonPage, error) {
	repository := queries.New(database.GetDatabase())
	offset := (page - 1) * 20
	quantity, err := repository.GetPokemonQuantity(context.Background())
	if err != nil {
		return PokemonPage{}, err
	}
	pokemons, err := repository.GetPokemonsPager(context.Background(), int32(offset))
	if err != nil {
		return PokemonPage{}, err
	}
	lastElementID := offset + 20
	element := PokemonPage{}
	if int64(lastElementID) >= quantity {
		element.Last = true
	}

	element.Pokemons = pokemons
	element.Elements = 20
	element.Page = page

	return element, nil
}
