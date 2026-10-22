import { beforeEach, describe, expect, test } from 'bun:test'
import { falsearFetch, json } from '../../test/fetch.ts'
import { ApiError } from './cliente.ts'
import { buscarPokemon, obtenerJuegos, obtenerLideres, obtenerPokemon, vaciarCacheCatalogo } from './catalogo.ts'

beforeEach(() => vaciarCacheCatalogo())

describe('catálogo', () => {
  test('rutas de cada función', async () => {
    const llamadas = falsearFetch(() => json([]))
    await obtenerJuegos()
    await obtenerLideres('pokemon platino/ñ')
    await buscarPokemon('gen4', '  PiKa ')
    await obtenerPokemon('gen4', 'Mr. Mime')
    expect(llamadas.map((l) => l.url)).toEqual([
      '/api/juegos',
      '/api/juegos/pokemon%20platino%2F%C3%B1/lideres',
      '/api/pokemon?gen=gen4&q=pika',
      '/api/pokemon/Mr.%20Mime?gen=gen4',
    ])
  })

  test('devuelve los datos de la respuesta', async () => {
    falsearFetch(() => json([{ id: 'platino', nombre: 'Platino', gen: 'gen4' }]))
    expect(await obtenerJuegos()).toEqual([{ id: 'platino', nombre: 'Platino', gen: 'gen4' }])
  })

  test('peticiones simultáneas comparten una sola llamada y la respuesta queda en caché', async () => {
    const llamadas = falsearFetch(() => json(['a']))
    const [a, b] = await Promise.all([obtenerJuegos(), obtenerJuegos()])
    expect(a).toBe(b)
    await obtenerJuegos()
    expect(llamadas).toHaveLength(1)
  })

  test('la caché es por ruta', async () => {
    const llamadas = falsearFetch(() => json([]))
    await buscarPokemon('gen4', 'pika')
    await buscarPokemon('gen5', 'pika')
    await buscarPokemon('gen4', 'PIKA')
    expect(llamadas).toHaveLength(2)
  })

  test('un fallo no se queda en caché: el siguiente intento vuelve a pedir', async () => {
    let n = 0
    const llamadas = falsearFetch(() => (++n === 1 ? json({ error: 'caído' }, 500) : json(['ok'])))
    await expect(obtenerJuegos()).rejects.toBeInstanceOf(ApiError)
    await Promise.resolve()
    expect(await obtenerJuegos()).toEqual(['ok'] as never)
    expect(llamadas).toHaveLength(2)
  })

  test('vaciarCacheCatalogo obliga a pedir de nuevo', async () => {
    const llamadas = falsearFetch(() => json([]))
    await obtenerJuegos()
    vaciarCacheCatalogo()
    await obtenerJuegos()
    expect(llamadas).toHaveLength(2)
  })
})
