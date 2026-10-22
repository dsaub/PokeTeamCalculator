import { describe, expect, test } from 'bun:test'
import { falsearEventSource, falsearFetch, json } from '../../test/fetch.ts'
import { ApiError } from './cliente.ts'
import { abrirEventos, cancelarSimulacion, consultarSimulacion, crearSimulacion } from './simulaciones.ts'
import type { PeticionSimulacion } from './tipos.ts'

const peticion: PeticionSimulacion = {
  gen: 'gen4',
  rival: { especie: 'Cranidos', nivel: 14, movimientos: ['Headbutt'] },
  candidatos: [{ especie: 'Monferno', movimientos: ['Mach Punch'] }],
}

describe('simulaciones', () => {
  test('crearSimulacion hace POST con el JSON de la petición', async () => {
    const llamadas = falsearFetch(() => json({ id: 3, estado: 'pendiente', posicion: 1, workerDisponible: true }, 202))
    const r = await crearSimulacion(peticion)
    expect(r).toEqual({ id: 3, estado: 'pendiente', posicion: 1, workerDisponible: true })
    const { url, init } = llamadas[0]!
    expect(url).toBe('/api/simulaciones')
    expect(init.method).toBe('POST')
    expect(init.headers).toMatchObject({ 'Content-Type': 'application/json', Accept: 'application/json' })
    expect(JSON.parse(init.body as string)).toEqual(peticion)
  })

  test('consultarSimulacion hace GET /simulaciones/:id', async () => {
    const llamadas = falsearFetch(() => json({ id: 9, estado: 'procesando' }))
    expect(await consultarSimulacion(9)).toEqual({ id: 9, estado: 'procesando' })
    expect(llamadas[0]!.url).toBe('/api/simulaciones/9')
    expect(llamadas[0]!.init.method).toBeUndefined()
  })

  test('cancelarSimulacion hace DELETE y acepta 204', async () => {
    const llamadas = falsearFetch(() => new Response(null, { status: 204 }))
    await expect(cancelarSimulacion(5)).resolves.toBeUndefined()
    expect(llamadas[0]!.url).toBe('/api/simulaciones/5')
    expect(llamadas[0]!.init.method).toBe('DELETE')
  })

  test('cancelarSimulacion propaga el 409', async () => {
    falsearFetch(() => json({ error: 'Ya ha terminado.' }, 409))
    const err = await cancelarSimulacion(5).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(409)
  })

  test('abrirEventos abre el SSE de la simulación', () => {
    const Fuente = falsearEventSource()
    abrirEventos(12)
    expect(Fuente.abiertas.map((f) => f.url)).toEqual(['/api/simulaciones/12/eventos'])
  })
})
