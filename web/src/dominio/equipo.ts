import { STATS, type Gen, type Stat } from '../api/tipos.ts'

export const MAX_EQUIPO = 6
export const MAX_MOVIMIENTOS = 4
export const MAX_EV = 252
export const MAX_EV_TOTAL = 510
export const MAX_IV = 31

/**
 * Un Pokémon del equipo tal como está en el formulario. Los campos numéricos se guardan como
 * texto (lo que hay en el input) y '' significa «sin indicar»: así la validación puede señalar
 * lo que está mal escrito y la petición omitir lo que está vacío.
 */
export interface MiembroEquipo {
  uid: string
  especie: string
  nombre: string
  tipos: string[]
  nivel: string
  movimientos: string[]
  habilidad: string
  objeto: string
  naturaleza: string
  evs: Record<Stat, string>
  ivs: Record<Stat, string>
}

export function numeroGen(gen: Gen): number {
  return Number(gen.slice(3))
}

/** Qué campos opcionales existen en cada generación. */
export function camposDeGen(gen: Gen) {
  const n = numeroGen(gen)
  return {
    habilidad: n >= 3,
    naturaleza: n >= 3,
    objeto: n >= 2,
    // En gen 1 el Ataque Especial y la Defensa Especial son un único stat «Especial»,
    // que Showdown lee de `spa`. Desde gen 2 son stats separados (`spa` y `spd`).
    stats: n >= 2 ? STATS : STATS.filter((s) => s !== 'spd'),
  }
}

const statsVacios = (): Record<Stat, string> => ({ hp: '', atk: '', def: '', spa: '', spd: '', spe: '' })

let contador = 0
export function nuevoUid(): string {
  contador += 1
  return `m${Date.now().toString(36)}${contador}`
}

export function nuevoMiembro(p: { especie: string; nombre: string; tipos: string[] }, nivel = ''): MiembroEquipo {
  return {
    uid: nuevoUid(),
    especie: p.especie,
    nombre: p.nombre,
    tipos: p.tipos,
    nivel,
    movimientos: [],
    habilidad: '',
    objeto: '',
    naturaleza: '',
    evs: statsVacios(),
    ivs: statsVacios(),
  }
}

/** Reconstruye un miembro a partir de datos de origen dudoso (p. ej. localStorage). */
export function sanearMiembro(raw: unknown): MiembroEquipo | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const texto = (v: unknown) => (typeof v === 'string' ? v : '')
  const textos = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [])
  const stats = (v: unknown) => {
    const base = statsVacios()
    if (v && typeof v === 'object') {
      for (const s of STATS) base[s] = texto((v as Record<string, unknown>)[s])
    }
    return base
  }
  const especie = texto(r.especie)
  if (!especie) return null
  return {
    uid: nuevoUid(),
    especie,
    nombre: texto(r.nombre) || especie,
    tipos: textos(r.tipos),
    nivel: texto(r.nivel),
    movimientos: textos(r.movimientos).slice(0, MAX_MOVIMIENTOS),
    habilidad: texto(r.habilidad),
    objeto: texto(r.objeto),
    naturaleza: texto(r.naturaleza),
    evs: stats(r.evs),
    ivs: stats(r.ivs),
  }
}
