import { afterAll, afterEach, beforeAll, describe, expect, test } from 'bun:test'
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { AddressInfo } from 'node:net'
import type { ViteDevServer } from 'vite'
import type { EstadoSimulacion, PeticionSimulacion, PokemonDetalle } from '../src/api/tipos.ts'
import { JUEGOS, LIDERES, MOVIMIENTOS, POKEMON } from './datos.ts'
import { mockApi, reiniciarMock } from './servidor.ts'

// Monta el middleware del plugin en un servidor HTTP real, como haría Vite en `/api`.

type Middleware = (req: IncomingMessage, res: ServerResponse, siguiente: (e?: unknown) => void) => void

let servidor: Server
let base: string

beforeAll(async () => {
  let manejador: Middleware | undefined
  const configurar = mockApi().configureServer as (s: ViteDevServer) => void
  configurar({
    middlewares: {
      use: (ruta: string, fn: Middleware) => {
        expect(ruta).toBe('/api')
        manejador = fn
      },
    },
  } as unknown as ViteDevServer)
  servidor = createServer((req, res) => {
    if (!req.url?.startsWith('/api')) {
      res.statusCode = 418
      return res.end()
    }
    req.url = req.url.slice(4) || '/'
    manejador!(req, res, (e) => {
      res.statusCode = 500
      res.end(String(e))
    })
  })
  await new Promise<void>((r) => servidor.listen(0, '127.0.0.1', r))
  base = `http://127.0.0.1:${(servidor.address() as AddressInfo).port}/api`
})

afterAll(() => servidor.close())
afterEach(() => reiniciarMock())

const pedir = (ruta: string, init?: RequestInit) => fetch(base + ruta, init)

const peticion: PeticionSimulacion = {
  gen: 'gen4',
  rival: { especie: 'Cranidos', nivel: 14, movimientos: ['Headbutt'] },
  candidatos: [
    { especie: 'Monferno', nivel: 20, movimientos: ['Mach Punch'] },
    { especie: 'Grotle', movimientos: ['Razor Leaf'] },
  ],
}

const crear = (cuerpo: unknown = peticion) =>
  pedir('/simulaciones', { method: 'POST', body: typeof cuerpo === 'string' ? cuerpo : JSON.stringify(cuerpo) })

/** Lee el SSE hasta que el servidor cierra y devuelve los eventos. */
async function leerEventos(res: Response): Promise<{ evento: string; datos: unknown }[]> {
  const texto = await res.text()
  return texto
    .split('\n\n')
    .filter(Boolean)
    .map((bloque) => {
      const [ev, data] = bloque.split('\n')
      return { evento: ev!.slice('event: '.length), datos: JSON.parse(data!.slice('data: '.length)) }
    })
}

describe('catálogo', () => {
  test('GET /juegos', async () => {
    const res = await pedir('/juegos')
    expect(res.status).toBe(200)
    expect(res.headers.get('content-type')).toContain('application/json')
    expect(await res.json()).toEqual(JUEGOS)
  })

  test('GET /juegos/:id/lideres', async () => {
    expect(await (await pedir('/juegos/platino/lideres')).json()).toEqual(LIDERES.platino!)
    expect((await pedir('/juegos/nada/lideres')).status).toBe(404)
  })

  test('GET /pokemon filtra por generación y texto', async () => {
    const gen4 = (await (await pedir('/pokemon?gen=gen4&q=onf')).json()) as { especie: string }[]
    expect(gen4.map((p) => p.especie)).toContain('Monferno')
    const gen1 = (await (await pedir('/pokemon?gen=gen1&q=onf')).json()) as unknown[]
    expect(gen1).toEqual([])
    const porNombre = (await (await pedir('/pokemon?gen=gen9&q=PIKA')).json()) as { especie: string }[]
    expect(porNombre.map((p) => p.especie)).toEqual(['Pikachu'])
  })

  test('GET /pokemon con menos de dos caracteres → 400', async () => {
    const res = await pedir('/pokemon?gen=gen4&q=m')
    expect(res.status).toBe(400)
    expect(await res.json()).toEqual({ error: 'q: debe tener al menos 2 caracteres' })
    expect((await pedir('/pokemon')).status).toBe(400)
  })

  test('GET /pokemon/:especie solo trae movimientos de la generación', async () => {
    const d = (await (await pedir('/pokemon/Charmander?gen=gen1')).json()) as PokemonDetalle
    expect(d.especie).toBe('Charmander')
    expect(d.habilidades[0]).toEqual({ id: 'Blaze', nombre: expect.any(String) })
    expect(d.movimientos.map((m) => m.id)).not.toContain('Metal Claw')
    expect(d.movimientos.find((m) => m.id === 'Growl')!.potencia).toBeNull()
    expect(d.movimientos.every((m) => m.tipo === m.tipo.toLowerCase())).toBe(true)
    const gen4 = (await (await pedir('/pokemon/charmander?gen=gen4')).json()) as PokemonDetalle
    expect(gen4.movimientos.map((m) => m.id)).toContain('Metal Claw')
    expect((await pedir('/pokemon/Mr.%20Mime')).status).toBe(200)
    expect((await pedir('/pokemon/Missingno')).status).toBe(404)
  })

  test('ruta o método desconocido → 404', async () => {
    expect((await pedir('/nada')).status).toBe(404)
    expect((await pedir('/juegos', { method: 'POST' })).status).toBe(404)
    expect((await pedir('/simulaciones/1', { method: 'PUT' })).status).toBe(404)
  })
})

describe('simulaciones', () => {
  test('POST con JSON inválido o sin candidatos → 400', async () => {
    let res = await crear('{roto')
    expect(res.status).toBe(400)
    expect(await res.json()).toEqual({ error: 'cuerpo: JSON inválido' })
    res = await crear({ ...peticion, candidatos: [] })
    expect(res.status).toBe(400)
    res = await crear({ gen: 'gen4' })
    expect(res.status).toBe(400)
  })

  test('POST → 202 en cola con posición creciente', async () => {
    const a = await crear()
    expect(a.status).toBe(202)
    expect(await a.json()).toEqual({ id: 1, estado: 'pendiente', posicion: 1, estimadoSegundos: 4, workerDisponible: true })
    expect(await (await crear()).json()).toMatchObject({ id: 2, posicion: 2, estimadoSegundos: 8 })
    expect(await (await pedir('/simulaciones/2')).json()).toEqual({ id: 2, estado: 'pendiente', posicion: 2, estimadoSegundos: 8 })
  })

  test('GET de una que no existe → 404', async () => {
    expect((await pedir('/simulaciones/99')).status).toBe(404)
    expect((await pedir('/simulaciones/99', { method: 'DELETE' })).status).toBe(404)
    expect((await pedir('/simulaciones/99/eventos')).status).toBe(404)
  })

  test('DELETE → 204 sin cuerpo; la segunda vez → 409', async () => {
    await crear()
    const res = await pedir('/simulaciones/1', { method: 'DELETE' })
    expect(res.status).toBe(204)
    expect(await res.text()).toBe('')
    expect(await (await pedir('/simulaciones/1')).json()).toEqual({ id: 1, estado: 'cancelado' })
    const otra = await pedir('/simulaciones/1', { method: 'DELETE' })
    expect(otra.status).toBe(409)
  })

  test('SSE de una cancelada: posición y cancelado, y cierra', async () => {
    await crear()
    await pedir('/simulaciones/1', { method: 'DELETE' })
    const res = await pedir('/simulaciones/1/eventos')
    expect(res.headers.get('content-type')).toContain('text/event-stream')
    expect((await leerEventos(res)).map((e) => e.evento)).toEqual(['posicion', 'cancelado'])
  })

  test(
    'recorrido completo: procesando → completado con resultados ordenados',
    async () => {
      await crear()
      const eventos = await leerEventos(await pedir('/simulaciones/1/eventos'))
      const nombres = eventos.map((e) => e.evento)
      expect(nombres[0]).toBe('posicion')
      expect(nombres).toContain('procesando')
      expect(nombres.at(-1)).toBe('completado')

      const estado = (await (await pedir('/simulaciones/1')).json()) as Extract<EstadoSimulacion, { estado: 'completado' }>
      expect(estado.estado).toBe('completado')
      const r = estado.resultado
      expect(r.rival).toBe('Cranidos (Nv. 14)')
      expect(r.gen).toBe('gen4')
      expect(r.resultados).toHaveLength(2)
      expect(r.resultados.find((x) => x.pokemon === 'Grotle')!.nivel).toBe(100)
      expect(r.resultados[0]!.winrate).toBeGreaterThanOrEqual(r.resultados[1]!.winrate)
      for (const x of r.resultados) expect(x.victorias + x.derrotas + x.empates).toBe(300)

      expect((await pedir('/simulaciones/1', { method: 'DELETE' })).status).toBe(409)
    },
    10_000,
  )
})

describe('datos del mock', () => {
  test('los movimientos de cada Pokémon existen', () => {
    for (const p of POKEMON) for (const m of p.movimientos) expect(MOVIMIENTOS[m], `${p.especie}: ${m}`).toBeDefined()
  })

  test('cada juego tiene líderes y sus equipos tienen movimientos', () => {
    for (const j of JUEGOS) {
      const lideres = LIDERES[j.id]
      expect(lideres?.length, j.id).toBeGreaterThan(0)
      for (const l of lideres!) {
        expect(l.equipo.length).toBeGreaterThan(0)
        for (const p of l.equipo) expect(p.movimientos.length).toBeGreaterThan(0)
      }
    }
  })

  test('especies sin repetir', () => {
    const especies = POKEMON.map((p) => p.especie)
    expect(new Set(especies).size).toBe(especies.length)
  })
})
