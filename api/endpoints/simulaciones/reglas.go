package simulaciones

import (
	"context"
)

// SanearPorGeneracion elimina campos que el worker rechaza en generaciones
// antiguas:
//   - gen 1: habilidad, naturaleza y objeto;
//   - gen 2: habilidad y naturaleza.
func SanearPorGeneracion(p *Peticion) {
	gen := generacionDe(p.Gen)
	if gen < 1 {
		return
	}
	sanear := func(pk *PokemonJSON) {
		if gen <= 2 {
			pk.Habilidad = ""
			pk.Naturaleza = ""
		}
		if gen == 1 {
			pk.Objeto = ""
		}
	}
	sanear(&p.Rival)
	for i := range p.Candidatos {
		sanear(&p.Candidatos[i])
	}
}

func generacionDe(gen string) int {
	if len(gen) != 4 || gen[:3] != "gen" || gen[3] < '1' || gen[3] > '9' {
		return 0
	}
	return int(gen[3] - '0')
}

func validarOpciones(op *Opciones) error {
	if op == nil {
		return nil
	}
	if op.AleatoriedadRival != nil && (*op.AleatoriedadRival < 0 || *op.AleatoriedadRival > 1) {
		return errorCampo("opciones.aleatoriedadRival", "debe estar entre 0 y 1")
	}
	if op.MaxTurnos != nil && (*op.MaxTurnos < 1 || *op.MaxTurnos > 1000) {
		return errorCampo("opciones.maxTurnos", "debe estar entre 1 y 1000")
	}
	return nil
}

func (s *Service) revisarHabilidades(ctx context.Context, p *Peticion) {
	if s.catalog == nil {
		return
	}
	gen := generacionDe(p.Gen)
	if gen < 3 {
		return
	}
	revisar := func(pk *PokemonJSON) {
		if pk.Habilidad == "" {
			return
		}
		existe, valida, err := s.catalog.HabilidadValida(ctx, gen, pk.Especie, pk.Habilidad)
		if err != nil {
			s.logger.Warn("catalogo_habilidades", "error", err)
			return
		}
		if existe && !valida {
			// El worker pondrá la primera habilidad de la especie.
			s.logger.Debug("habilidad_descartada", "gen", p.Gen, "especie", pk.Especie, "habilidad", pk.Habilidad)
			pk.Habilidad = ""
		}
	}
	revisar(&p.Rival)
	for i := range p.Candidatos {
		revisar(&p.Candidatos[i])
	}
}
