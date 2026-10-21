package catalogo

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ErrorValidacion struct {
	Campo   string
	Mensaje string
}

func (e *ErrorValidacion) Error() string {
	if e.Campo == "" {
		return e.Mensaje
	}
	return e.Campo + ": " + e.Mensaje
}

var genValida = regexp.MustCompile(`^gen[1-9]$`)

type Service struct {
	repo Repo
}

func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

func (s *Service) Juegos(ctx context.Context) ([]Juego, error) {
	juegos, err := s.repo.Juegos(ctx)
	if err != nil {
		return nil, err
	}
	if juegos == nil {
		juegos = []Juego{}
	}
	return juegos, nil
}

func (s *Service) Lideres(ctx context.Context, juego string) ([]Lider, error) {
	juegos, err := s.repo.Juegos(ctx)
	if err != nil {
		return nil, err
	}
	for _, j := range juegos {
		if j.ID == juego {
			gen := int(j.Gen[3] - '0')
			lideres, err := s.repo.Lideres(ctx, j.VersionGroup, gen)
			if err != nil {
				return nil, err
			}
			if lideres == nil {
				lideres = []Lider{}
			}
			return lideres, nil
		}
	}
	return nil, ErrNoEncontrado
}

func (s *Service) BuscarPokemon(ctx context.Context, gen, q string) ([]PokemonResumen, error) {
	g, err := validarGen(gen)
	if err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 2 {
		return nil, &ErrorValidacion{Campo: "q", Mensaje: "debe tener al menos 2 caracteres"}
	}
	resultados, err := s.repo.BuscarPokemon(ctx, g, q, 20)
	if err != nil {
		return nil, err
	}
	if resultados == nil {
		resultados = []PokemonResumen{}
	}
	return resultados, nil
}

func (s *Service) Pokemon(ctx context.Context, gen, especie string) (*PokemonDetalle, error) {
	g, err := validarGen(gen)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(especie) == "" {
		return nil, ErrNoEncontrado
	}
	return s.repo.Pokemon(ctx, g, especie)
}

func validarGen(gen string) (int, error) {
	if !genValida.MatchString(gen) {
		return 0, &ErrorValidacion{Campo: "gen", Mensaje: "debe ser gen1, gen2, …, gen9"}
	}
	return int(gen[3] - '0'), nil
}
