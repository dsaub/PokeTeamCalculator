import type { IncomingMessage, ServerResponse } from 'node:http'
import type { Plugin } from 'vite'
import type { EstadoSimulacion, PeticionSimulacion, PokemonDetalle, PokemonResumen, Resultado } from '../src/api/tipos.ts'
import { JUEGOS, LIDERES, MOVIMIENTOS, POKEMON } from './datos.ts'

// API de mentira para desarrollo (MOCK=1). No usa la base de datos ni el
// worker: reproduce el contrato HTTP y SSE con datos reducidos de `datos.ts`.

interface TrabajoMock {
  id: number
  peticion: PeticionSimulacion
  estado: 'pendiente' | 'procesando' | 'completado' | 'cancelado'
  creado: number
  resultado?: Resultado
}

const trabajos = new Map<number, TrabajoMock>()
const esperas = new Map<number, ReturnType<typeof setTimeout>[]>()
let siguienteId = 1

function json(res: ServerResponse, codigo: number, cuerpo: unknown): void {
  res.statusCode = codigo
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(cuerpo))
}

function leerCuerpo(req: IncomingMessage): Promise<string> {
  return new Promise((resolver, rechazar) => {
    let datos = ''
    req.setEncoding('utf8')
    req.on('data', (parte) => {
      datos += parte
      if (datos.length > 64 * 1024) rechazar(new Error('cuerpo demasiado grande'))
    })
    req.on('end', () => resolver(datos))
    req.on('error', rechazar)
  })
}

function posicion(id: number): number {
  let pos = 0
  for (const t of trabajos.values()) {
    if (t.estado === 'pendiente' && t.id <= id) pos++
  }
  return Math.max(1, pos)
}

function estadoDe(t: TrabajoMock): EstadoSimulacion {
  if (t.estado === 'pendiente') {
    return { id: t.id, estado: 'pendiente', posicion: posicion(t.id), estimadoSegundos: posicion(t.id) * 4 }
  }
  if (t.estado === 'procesando') return { id: t.id, estado: 'procesando' }
  if (t.estado === 'cancelado') return { id: t.id, estado: 'cancelado' }
  return { id: t.id, estado: 'completado', resultado: t.resultado! }
}

/** Winrate determinista y distinto por Pokémon, solo para la demo. */
function winrate(especie: string): number {
  let h = 0
  for (const c of especie) h = (h * 31 + c.charCodeAt(0)) % 1000
  return Math.min(1, 0.35 + h / 1500)
}

function completar(t: TrabajoMock): void {
  const resultados = t.peticion.candidatos.map((c) => {
    const w = winrate(c.especie)
    const victorias = Math.round(w * 300)
    return {
      pokemon: c.especie,
      nivel: c.nivel ?? 100,
      victorias,
      derrotas: 300 - victorias,
      empates: 0,
      winrate: victorias / 300,
      psMedioVictoria: Math.min(1, w + 0.1),
      turnosMedio: 4 + (1 - w) * 8,
    }
  })
  resultados.sort((a, b) => b.winrate - a.winrate || b.psMedioVictoria - a.psMedioVictoria || a.turnosMedio - b.turnosMedio)
  t.estado = 'completado'
  t.resultado = {
    gen: t.peticion.gen,
    rival: `${t.peticion.rival.especie} (Nv. ${t.peticion.rival.nivel})`,
    simulaciones: 300,
    resultados,
    ms: 1240,
  }
}

function programar(t: TrabajoMock): void {
  const timers = [
    setTimeout(() => {
      if (t.estado === 'pendiente') t.estado = 'procesando'
    }, 1200),
    setTimeout(() => {
      if (t.estado === 'pendiente' || t.estado === 'procesando') completar(t)
    }, 3200),
  ]
  esperas.set(t.id, timers)
}

function detalle(especie: string, gen: number): PokemonDetalle | null {
  const p = POKEMON.find((x) => x.especie.toLowerCase() === especie.toLowerCase())
  if (!p) return null
  const movimientos = p.movimientos
    .map((id) => MOVIMIENTOS[id])
    .filter((m): m is NonNullable<typeof m> => Boolean(m) && m!.gen <= gen)
    .map((m) => ({
      id: m.id,
      nombre: m.nombre,
      tipo: m.tipo.toLowerCase(),
      categoria: m.categoria,
      potencia: m.categoria === 'status' ? null : m.potencia,
    }))
  return {
    especie: p.especie,
    nombre: p.nombre,
    tipos: p.tipos,
    habilidades: p.habilidades.map(([id, nombre]) => ({ id, nombre })),
    movimientos: movimientos.slice(0, 8),
  }
}

function resumen(p: (typeof POKEMON)[number]): PokemonResumen {
  return { especie: p.especie, nombre: p.nombre, tipos: p.tipos }
}

function api(req: IncomingMessage, res: ServerResponse): void {
  const url = new URL(req.url ?? '/', 'http://mock')
  const ruta = url.pathname
  const metodo = req.method ?? 'GET'

  if (metodo === 'GET' && ruta === '/juegos') {
    return json(res, 200, JUEGOS)
  }

  const lideres = /^\/juegos\/([^/]+)\/lideres$/.exec(ruta)
  if (metodo === 'GET' && lideres) {
    const lista = LIDERES[decodeURIComponent(lideres[1]!)]
    return lista ? json(res, 200, lista) : json(res, 404, { error: 'No encontrado' })
  }

  if (metodo === 'GET' && ruta === '/pokemon') {
    const gen = Number((url.searchParams.get('gen') ?? 'gen9').slice(3))
    const q = (url.searchParams.get('q') ?? '').toLowerCase()
    if (q.length < 2) return json(res, 400, { error: 'q: debe tener al menos 2 caracteres' })
    const lista = POKEMON.filter(
      (p) => p.gen <= gen && (p.especie.toLowerCase().includes(q) || p.nombre.toLowerCase().includes(q)),
    )
      .slice(0, 20)
      .map(resumen)
    return json(res, 200, lista)
  }

  const pokemon = /^\/pokemon\/([^/]+)$/.exec(ruta)
  if (metodo === 'GET' && pokemon) {
    const gen = Number((url.searchParams.get('gen') ?? 'gen9').slice(3))
    const d = detalle(decodeURIComponent(pokemon[1]!), gen)
    return d ? json(res, 200, d) : json(res, 404, { error: 'No encontrado' })
  }

  if (metodo === 'POST' && ruta === '/simulaciones') {
    void leerCuerpo(req).then((texto) => {
      let peticion: PeticionSimulacion
      try {
        peticion = JSON.parse(texto) as PeticionSimulacion
      } catch {
        json(res, 400, { error: 'cuerpo: JSON inválido' })
        return
      }
      if (!peticion.gen || !peticion.rival || !Array.isArray(peticion.candidatos) || peticion.candidatos.length === 0) {
        json(res, 400, { error: 'candidatos: debe haber entre 1 y 6' })
        return
      }
      const t: TrabajoMock = { id: siguienteId++, peticion, estado: 'pendiente', creado: Date.now() }
      trabajos.set(t.id, t)
      programar(t)
      json(res, 202, {
        id: t.id,
        estado: 'pendiente',
        posicion: posicion(t.id),
        estimadoSegundos: posicion(t.id) * 4,
        workerDisponible: true,
      })
    })
    return
  }

  const estado = /^\/simulaciones\/(\d+)$/.exec(ruta)
  if (estado) {
    const t = trabajos.get(Number(estado[1]))
    if (!t) return json(res, 404, { error: 'Simulación no encontrada' })
    if (metodo === 'GET') return json(res, 200, estadoDe(t))
    if (metodo === 'DELETE') {
      if (t.estado === 'completado' || t.estado === 'cancelado') {
        return json(res, 409, { error: 'La simulación ya había terminado' })
      }
      t.estado = 'cancelado'
      for (const timer of esperas.get(t.id) ?? []) clearTimeout(timer)
      res.statusCode = 204
      return void res.end()
    }
  }

  const eventos = /^\/simulaciones\/(\d+)\/eventos$/.exec(ruta)
  if (metodo === 'GET' && eventos) {
    const t = trabajos.get(Number(eventos[1]))
    if (!t) return json(res, 404, { error: 'Simulación no encontrada' })
    res.writeHead(200, {
      'Content-Type': 'text/event-stream; charset=utf-8',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    })
    const enviar = (evento: string, datos: unknown) => {
      res.write(`event: ${evento}\ndata: ${JSON.stringify(datos)}\n\n`)
    }
    enviar('posicion', { posicion: posicion(t.id), estimadoSegundos: posicion(t.id) * 4 })
    const revisar = setInterval(() => {
      if (t.estado === 'procesando') {
        enviar('procesando', {})
      }
      if (t.estado === 'completado') {
        enviar('completado', { resultado: t.resultado })
        clearInterval(revisar)
        res.end()
      }
      if (t.estado === 'cancelado') {
        enviar('cancelado', {})
        clearInterval(revisar)
        res.end()
      }
    }, 400)
    req.on('close', () => clearInterval(revisar))
    return
  }

  json(res, 404, { error: 'No encontrado' })
}

/** Solo para tests: olvida los trabajos y cancela sus temporizadores. */
export function reiniciarMock(): void {
  for (const timers of esperas.values()) for (const t of timers) clearTimeout(t)
  esperas.clear()
  trabajos.clear()
  siguienteId = 1
}

export function mockApi(): Plugin {
  return {
    name: 'contrataque-mock-api',
    configureServer(servidor) {
      servidor.middlewares.use('/api', (req, res, siguiente) => {
        try {
          api(req, res)
        } catch (e) {
          siguiente(e)
        }
      })
    },
  }
}
