import type { PeticionSimulacion } from '../api/tipos.ts'
import { MAX_EQUIPO, sanearMiembro, type MiembroEquipo } from './equipo.ts'

// Persistencia en localStorage. Puede no existir, lanzar al acceder (modo privado, cookies
// bloqueadas) o estar llena: cada acceso va en try/catch y un fallo nunca rompe la interfaz.

const PREFIJO = 'contrataque:v2'

/** Devuelve localStorage o null; el mero acceso a `window.localStorage` puede lanzar. */
export type ObtenerAlmacen = () => Storage | null
const almacenNavegador: ObtenerAlmacen = () => globalThis.localStorage ?? null

function leer(clave: string, almacen: ObtenerAlmacen): unknown {
  try {
    const texto = almacen()?.getItem(clave)
    return texto ? JSON.parse(texto) : null
  } catch {
    return null
  }
}

function escribir(clave: string, valor: unknown, almacen: ObtenerAlmacen): boolean {
  try {
    const a = almacen()
    if (!a) return false
    a.setItem(clave, JSON.stringify(valor))
    return true
  } catch {
    return false
  }
}

// ---------- Equipo por juego ----------

export function cargarEquipo(juego: string, almacen = almacenNavegador): MiembroEquipo[] {
  const datos = leer(`${PREFIJO}:equipo:${juego}`, almacen)
  if (!Array.isArray(datos)) return []
  return datos
    .map(sanearMiembro)
    .filter((m): m is MiembroEquipo => m !== null)
    .slice(0, MAX_EQUIPO)
}

/** Devuelve false si no se pudo guardar (el equipo sigue en memoria). */
export function guardarEquipo(juego: string, equipo: MiembroEquipo[], almacen = almacenNavegador): boolean {
  // El uid es solo para React; no se guarda.
  const sinUid = equipo.map(({ uid: _uid, ...resto }) => resto)
  return escribir(`${PREFIJO}:equipo:${juego}`, sinUid, almacen)
}

// ---------- Último juego elegido ----------

export function cargarUltimoJuego(almacen = almacenNavegador): string | null {
  const v = leer(`${PREFIJO}:juego`, almacen)
  return typeof v === 'string' ? v : null
}

export function guardarUltimoJuego(juego: string, almacen = almacenNavegador): void {
  escribir(`${PREFIJO}:juego`, juego, almacen)
}

// ---------- Contexto de cada simulación ----------

/**
 * Lo que la interfaz sabe de una simulación lanzada desde este navegador y la API no devuelve:
 * de qué juego y líder era, los nombres en español y la petición (para «siguiente Pokémon» y
 * «volver a simular»). Un enlace abierto en otro navegador no lo tiene.
 */
export interface ContextoSimulacion {
  juego: string
  lider: string
  rival: number
  peticion: PeticionSimulacion
  /** especie → nombre para mostrar, del rival y de los candidatos. */
  nombres: Record<string, string>
}

const MAX_CONTEXTOS = 20

export function guardarContexto(id: number, ctx: ContextoSimulacion, almacen = almacenNavegador): void {
  const clave = `${PREFIJO}:simulaciones`
  const previos = leer(clave, almacen)
  const lista = Array.isArray(previos) ? (previos as [number, ContextoSimulacion][]) : []
  const nueva = [[id, ctx] as const, ...lista.filter(([i]) => i !== id)].slice(0, MAX_CONTEXTOS)
  escribir(clave, nueva, almacen)
}

export function cargarContexto(id: number, almacen = almacenNavegador): ContextoSimulacion | null {
  const lista = leer(`${PREFIJO}:simulaciones`, almacen)
  if (!Array.isArray(lista)) return null
  const encontrado = lista.find((e): e is [number, ContextoSimulacion] => Array.isArray(e) && e[0] === id)
  const ctx = encontrado?.[1]
  if (!ctx || typeof ctx.juego !== 'string' || !ctx.peticion || typeof ctx.nombres !== 'object') return null
  return ctx
}
