import { pedir } from './cliente.ts'
import type { Gen, Juego, Lider, PokemonDetalle, PokemonResumen } from './tipos.ts'

// Las respuestas del catálogo no cambian durante la sesión: se guardan en memoria por ruta.
// Se guarda la promesa para no repetir peticiones simultáneas; si falla, se olvida y el
// siguiente intento vuelve a pedirla.
const cache = new Map<string, Promise<unknown>>()

function pedirCacheado<T>(ruta: string): Promise<T> {
  let promesa = cache.get(ruta) as Promise<T> | undefined
  if (!promesa) {
    promesa = pedir<T>(ruta).then((r) => r.datos)
    promesa.catch(() => cache.delete(ruta))
    cache.set(ruta, promesa)
  }
  return promesa
}

/** Solo para tests. */
export function vaciarCacheCatalogo(): void {
  cache.clear()
}

export function obtenerJuegos(): Promise<Juego[]> {
  return pedirCacheado('/juegos')
}

export function obtenerLideres(juego: string): Promise<Lider[]> {
  return pedirCacheado(`/juegos/${encodeURIComponent(juego)}/lideres`)
}

export function buscarPokemon(gen: Gen, q: string): Promise<PokemonResumen[]> {
  const params = new URLSearchParams({ gen, q: q.trim().toLowerCase() })
  return pedirCacheado(`/pokemon?${params}`)
}

export function obtenerPokemon(gen: Gen, especie: string): Promise<PokemonDetalle> {
  return pedirCacheado(`/pokemon/${encodeURIComponent(especie)}?${new URLSearchParams({ gen })}`)
}
