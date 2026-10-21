package pokemons

import (
	"PokeTeamCalculator/database"
	"PokeTeamCalculator/queries"
	"context"
	"errors"
)

const (
	pageSize  = 20
	maxPagina = 1_000_000
)

// ErrPaginaInvalida indica que el número de página no es válido.
var ErrPaginaInvalida = errors.New("page must be greater than 0")

func ServiceGetPokemons(page int) (PokemonPage, error) {
	if page < 1 {
		return PokemonPage{}, ErrPaginaInvalida
	}

	repository := queries.New(database.GetDatabase())
	quantity, err := repository.GetPokemonQuantity(context.Background())
	if err != nil {
		return PokemonPage{}, err
	}

	totalPages := 0
	if quantity > 0 {
		totalPages = int((quantity + pageSize - 1) / pageSize)
	}

	salida := PokemonPage{
		Page:       page,
		PageSize:   pageSize,
		Total:      quantity,
		TotalPages: totalPages,
		Pokemons:   []queries.GetPokemonsPagerRow{},
	}

	if page > maxPagina {
		salida.Last = true
		return salida, nil
	}
	offset := int64(page-1) * pageSize
	if offset >= quantity {
		// Página fuera de rango: array vacío, no null, y sin consultar la base.
		salida.Last = true
		return salida, nil
	}

	pokemons, err := repository.GetPokemonsPager(context.Background(), int32(offset))
	if err != nil {
		return PokemonPage{}, err
	}
	if pokemons != nil {
		salida.Pokemons = pokemons
	}
	salida.Last = offset+int64(len(salida.Pokemons)) >= quantity
	return salida, nil
}
