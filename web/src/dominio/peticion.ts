import type {
  CandidatoPeticion,
  Gen,
  PeticionSimulacion,
  PokemonLider,
  RivalPeticion,
  Stat,
  Stats,
} from '../api/tipos.ts'
import { camposDeGen, type MiembroEquipo } from './equipo.ts'

function stats(valores: Record<Stat, string>, visibles: readonly Stat[]): Stats | undefined {
  const out: Stats = {}
  for (const s of visibles) {
    // Un 0 escrito se envía: un IV que falta vale 31, no 0.
    if (valores[s].trim() !== '') out[s] = Number(valores[s])
  }
  return Object.keys(out).length > 0 ? out : undefined
}

/** El rival se copia del equipo del líder; solo se quitan los campos de visualización. */
export function construirRival(gen: Gen, p: PokemonLider): RivalPeticion {
  const campos = camposDeGen(gen)
  const rival: RivalPeticion = { especie: p.especie, nivel: p.nivel, movimientos: [...p.movimientos] }
  if (campos.habilidad && p.habilidad) rival.habilidad = p.habilidad
  if (campos.objeto && p.objeto) rival.objeto = p.objeto
  return rival
}

/**
 * Candidato tal como lo espera la API: `especie` en formato Showdown (nunca `nombre`), y los
 * campos opcionales sin rellenar o que no existen en la generación, omitidos.
 * Presupone un miembro válido (ver `validarEquipo`).
 */
export function construirCandidato(gen: Gen, m: MiembroEquipo): CandidatoPeticion {
  const campos = camposDeGen(gen)
  const c: CandidatoPeticion = { especie: m.especie, movimientos: [...m.movimientos] }
  if (m.nivel.trim() !== '') c.nivel = Number(m.nivel)
  if (campos.habilidad && m.habilidad) c.habilidad = m.habilidad
  if (campos.objeto && m.objeto.trim()) c.objeto = m.objeto.trim()
  if (campos.naturaleza && m.naturaleza) c.naturaleza = m.naturaleza
  const evs = stats(m.evs, campos.stats)
  if (evs) c.evs = evs
  const ivs = stats(m.ivs, campos.stats)
  if (ivs) c.ivs = ivs
  return c
}

export function construirPeticion(gen: Gen, rival: PokemonLider, equipo: MiembroEquipo[]): PeticionSimulacion {
  return {
    gen,
    rival: construirRival(gen, rival),
    candidatos: equipo.map((m) => construirCandidato(gen, m)),
  }
}
