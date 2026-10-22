import { describe, expect, test } from 'bun:test'
import { falsearFetch, json } from '../../test/fetch.ts'
import { API_BASE, ApiError, pedir } from './cliente.ts'

async function errorDe(p: Promise<unknown>): Promise<ApiError> {
  try {
    await p
  } catch (err) {
    expect(err).toBeInstanceOf(ApiError)
    return err as ApiError
  }
  throw new Error('se esperaba un ApiError')
}

describe('ApiError', () => {
  test('guarda estado, mensaje e id de la simulación', () => {
    const e = new ApiError(429, 'ya hay una', 7)
    expect(e.name).toBe('ApiError')
    expect(e.status).toBe(429)
    expect(e.message).toBe('ya hay una')
    expect(e.idSimulacion).toBe(7)
    expect(e).toBeInstanceOf(Error)
  })
})

describe('pedir', () => {
  test('añade /api y Accept, y conserva las cabeceras propias', async () => {
    const llamadas = falsearFetch(() => json({ ok: true }))
    const r = await pedir<{ ok: boolean }>('/juegos', { method: 'POST', headers: { 'X-Prueba': '1' } })
    expect(r).toEqual({ status: 200, datos: { ok: true } })
    expect(llamadas[0]!.url).toBe(`${API_BASE}/juegos`)
    expect(llamadas[0]!.init.method).toBe('POST')
    expect(llamadas[0]!.init.headers).toEqual({ Accept: 'application/json', 'X-Prueba': '1' })
  })

  test('204 → datos undefined sin leer el cuerpo', async () => {
    falsearFetch(() => new Response(null, { status: 204 }))
    expect(await pedir('/x')).toEqual({ status: 204, datos: undefined })
  })

  test('fallo de red → ApiError con estado 0', async () => {
    falsearFetch(() => {
      throw new TypeError('Failed to fetch')
    })
    const e = await errorDe(pedir('/x'))
    expect(e.status).toBe(0)
    expect(e.message).toContain('No se pudo conectar')
  })

  test('AbortError se relanza tal cual', async () => {
    const abortado = new DOMException('cancelada', 'AbortError')
    falsearFetch(() => {
      throw abortado
    })
    await expect(pedir('/x')).rejects.toBe(abortado)
  })

  test('usa el mensaje y el id del cuerpo de error', async () => {
    falsearFetch(() => json({ error: 'Ya tienes una simulación en curso.', id: 42 }, 429))
    const e = await errorDe(pedir('/x'))
    expect(e.status).toBe(429)
    expect(e.message).toBe('Ya tienes una simulación en curso.')
    expect(e.idSimulacion).toBe(42)
  })

  test('id que no es número se ignora', async () => {
    falsearFetch(() => json({ error: 'mal', id: '42' }, 400))
    const e = await errorDe(pedir('/x'))
    expect(e.message).toBe('mal')
    expect(e.idSimulacion).toBeUndefined()
  })

  const genericos: [number, string][] = [
    [404, 'No se ha encontrado'],
    [429, 'demasiadas simulaciones'],
    [500, 'no está disponible'],
    [503, 'no está disponible'],
    [418, 'error (418)'],
  ]
  for (const [status, texto] of genericos) {
    test(`cuerpo no JSON con ${status} → mensaje genérico`, async () => {
      falsearFetch(() => new Response('<html>proxy</html>', { status }))
      const e = await errorDe(pedir('/x'))
      expect(e.status).toBe(status)
      expect(e.message).toContain(texto)
    })
  }

  test('JSON sin `error` o con `error` vacío → mensaje genérico', async () => {
    falsearFetch(() => json({ error: '' }, 404))
    expect((await errorDe(pedir('/x'))).message).toContain('No se ha encontrado')
    falsearFetch(() => json({ otra: 1 }, 500))
    expect((await errorDe(pedir('/x'))).message).toContain('no está disponible')
  })
})
