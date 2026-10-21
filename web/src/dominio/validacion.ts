import type { Gen, Stat } from '../api/tipos.ts'
import {
  camposDeGen,
  MAX_EQUIPO,
  MAX_EV,
  MAX_EV_TOTAL,
  MAX_IV,
  MAX_MOVIMIENTOS,
  type MiembroEquipo,
} from './equipo.ts'

/** Errores de un miembro por campo. Las claves de stats son `evs.atk`, `ivs.hp`… */
export type ErroresMiembro = Partial<Record<string, string>>

export interface ResultadoValidacion {
  valido: boolean
  /** Error del equipo en conjunto (vacío o demasiado grande). */
  general?: string
  porMiembro: ErroresMiembro[]
}

const ENTERO = /^\d+$/

function validarEntero(valor: string, min: number, max: number): string | undefined {
  const v = valor.trim()
  if (v === '') return undefined
  if (!ENTERO.test(v) || Number(v) < min || Number(v) > max) return `Escribe un número entero entre ${min} y ${max}.`
  return undefined
}

/**
 * `aprendibles` (opcional): ids de los movimientos que puede aprender en esta generación. Si se
 * pasa, también se comprueba que los elegidos estén entre ellos (útil al recuperar un equipo
 * guardado).
 */
export function validarMiembro(gen: Gen, m: MiembroEquipo, aprendibles?: ReadonlySet<string>): ErroresMiembro {
  const errores: ErroresMiembro = {}
  const campos = camposDeGen(gen)

  if (!m.especie) errores.especie = 'Elige un Pokémon.'

  const nivel = validarEntero(m.nivel, 1, 100)
  if (nivel) errores.nivel = nivel

  if (m.movimientos.length === 0) errores.movimientos = 'Elige al menos un movimiento.'
  else if (m.movimientos.length > MAX_MOVIMIENTOS) errores.movimientos = `Como mucho ${MAX_MOVIMIENTOS} movimientos.`
  else if (new Set(m.movimientos).size !== m.movimientos.length) errores.movimientos = 'Hay movimientos repetidos.'
  else if (aprendibles && m.movimientos.some((mv) => !aprendibles.has(mv)))
    errores.movimientos = 'Algún movimiento no lo puede aprender en este juego.'

  let totalEvs = 0
  for (const s of campos.stats) {
    const ev = validarEntero(m.evs[s], 0, MAX_EV)
    if (ev) errores[`evs.${s}`] = ev
    else totalEvs += Number(m.evs[s].trim() || 0)
    const iv = validarEntero(m.ivs[s], 0, MAX_IV)
    if (iv) errores[`ivs.${s}`] = iv
  }
  if (totalEvs > MAX_EV_TOTAL) errores.evsTotal = `Los EVs suman ${totalEvs}; el máximo es ${MAX_EV_TOTAL}.`

  return errores
}

export function totalEvs(gen: Gen, evs: Record<Stat, string>): number {
  return camposDeGen(gen).stats.reduce((suma, s) => suma + (ENTERO.test(evs[s].trim()) ? Number(evs[s]) : 0), 0)
}

export function validarEquipo(
  gen: Gen,
  equipo: MiembroEquipo[],
  aprendibles: ReadonlyMap<string, ReadonlySet<string>> = new Map(),
): ResultadoValidacion {
  const porMiembro = equipo.map((m) => validarMiembro(gen, m, aprendibles.get(m.especie)))
  let general: string | undefined
  if (equipo.length === 0) general = 'Añade al menos un Pokémon a tu equipo.'
  else if (equipo.length > MAX_EQUIPO) general = `El equipo admite como mucho ${MAX_EQUIPO} Pokémon.`
  const valido = !general && porMiembro.every((e) => Object.keys(e).length === 0)
  return { valido, general, porMiembro }
}
