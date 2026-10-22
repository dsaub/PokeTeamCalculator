import { beforeEach, describe, expect, test } from 'bun:test'
import { ApiError } from '../api/cliente.ts'
import type { EstadoSimulacion, Resultado } from '../api/tipos.ts'
import type { Accion } from './maquina.ts'
import { seguirSimulacion, type FuenteEventos } from './seguimiento.ts'

const resultado: Resultado = { gen: 'gen4', rival: 'Cranidos (Nv. 14)', simulaciones: 300, resultados: [], ms: 1 }

class FuenteFalsa implements FuenteEventos {
  readyState = 0
  cerrada = false
  private oyentes = new Map<string, ((e: Event) => void)[]>()
  addEventListener(tipo: string, fn: (e: Event) => void) {
    this.oyentes.set(tipo, [...(this.oyentes.get(tipo) ?? []), fn])
  }
  close() {
    this.cerrada = true
    this.readyState = 2
  }
  emitir(tipo: string, data?: unknown) {
    const e = data === undefined ? new Event(tipo) : new MessageEvent(tipo, { data: JSON.stringify(data) })
    for (const fn of this.oyentes.get(tipo) ?? []) fn(e)
  }
  abrir() {
    this.readyState = 1
    this.emitir('open')
  }
  /** Corte de conexión: EventSource dispara `error` sin datos y queda reconectando. */
  cortar(definitivo = false) {
    this.readyState = definitivo ? 2 : 0
    this.emitir('error')
  }
}

/** Temporizadores manuales: `avanzar()` ejecuta lo programado. */
function reloj() {
  let pendientes: (() => void)[] = []
  return {
    programar: (fn: () => void) => {
      pendientes.push(fn)
      return fn
    },
    desprogramar: (t: unknown) => {
      pendientes = pendientes.filter((f) => f !== t)
    },
    avanzar: async () => {
      const ahora = pendientes
      pendientes = []
      for (const fn of ahora) fn()
      await Promise.resolve()
      await Promise.resolve()
    },
    get cuantos() {
      return pendientes.length
    },
  }
}

const esperar = () => new Promise((r) => setTimeout(r, 0))

let acciones: Accion[]
let fuentes: FuenteFalsa[]
let respuestas: (EstadoSimulacion | Error)[]
let consultas: number
let r: ReturnType<typeof reloj>

function seguir() {
  return seguirSimulacion(
    7,
    (a) => acciones.push(a),
    {
      abrirEventos: () => {
        const f = new FuenteFalsa()
        fuentes.push(f)
        return f
      },
      consultar: async () => {
        consultas++
        const siguiente = respuestas.shift() ?? { id: 7, estado: 'procesando' }
        if (siguiente instanceof Error) throw siguiente
        return siguiente
      },
      programar: r.programar,
      desprogramar: r.desprogramar,
    },
    { maxFallos: 3, intervaloSondeoMs: 3000 },
  )
}

beforeEach(() => {
  acciones = []
  fuentes = []
  respuestas = []
  consultas = 0
  r = reloj()
})

const tipos = () => acciones.map((a) => (a.tipo === 'evento' ? `evento:${a.evento.nombre}` : a.tipo === 'conexion' ? `conexion:${a.conexion}` : a.tipo))

describe('SSE', () => {
  test('traduce los eventos y cierra tras completado', () => {
    seguir()
    const f = fuentes[0]
    f.abrir()
    f.emitir('posicion', { posicion: 2, estimadoSegundos: 14 })
    f.emitir('procesando', {})
    f.emitir('completado', { resultado })
    expect(tipos()).toEqual(['conexion:conectando', 'conexion:conectado', 'evento:posicion', 'evento:procesando', 'evento:completado'])
    expect(acciones[2]).toEqual({ tipo: 'evento', evento: { nombre: 'posicion', datos: { posicion: 2, estimadoSegundos: 14 } } })
    expect(f.cerrada).toBe(true)
  })

  test('el evento error del servidor (con data) es un fin, no un corte', () => {
    seguir()
    fuentes[0].emitir('error', { error: 'Candidato 1: no existe' })
    expect(acciones.at(-1)).toEqual({ tipo: 'evento', evento: { nombre: 'error', datos: { error: 'Candidato 1: no existe' } } })
    expect(fuentes[0].cerrada).toBe(true)
  })

  test('cancelado cierra la conexión', () => {
    seguir()
    fuentes[0].emitir('cancelado', {})
    expect(fuentes[0].cerrada).toBe(true)
  })

  test('detener cierra la conexión y no despacha más', () => {
    const detener = seguir()
    detener()
    fuentes[0].emitir('posicion', { posicion: 1, estimadoSegundos: 7 })
    expect(fuentes[0].cerrada).toBe(true)
    expect(tipos()).toEqual(['conexion:conectando'])
  })

  test('un evento con JSON roto se ignora', () => {
    seguir()
    for (const fn of ['posicion']) {
      const e = new MessageEvent(fn, { data: '{roto' })
      ;(fuentes[0] as unknown as { oyentes: Map<string, ((e: Event) => void)[]> }).oyentes.get(fn)?.forEach((h) => h(e))
    }
    expect(tipos()).toEqual(['conexion:conectando'])
    expect(fuentes[0].cerrada).toBe(false)
  })
})

describe('reconexión y sondeo', () => {
  test('un corte aislado: reconectando, y al volver se recupera', () => {
    seguir()
    const f = fuentes[0]
    f.abrir()
    f.cortar()
    expect(acciones.at(-1)).toEqual({ tipo: 'conexion', conexion: 'reconectando' })
    f.abrir()
    expect(acciones.at(-1)).toEqual({ tipo: 'conexion', conexion: 'conectado' })
    expect(f.cerrada).toBe(false)
    expect(consultas).toBe(0)
  })

  test('los fallos se reinician al reconectar: no pasa a sondeo con cortes espaciados', () => {
    seguir()
    const f = fuentes[0]
    for (let i = 0; i < 5; i++) {
      f.cortar()
      f.cortar()
      f.abrir()
    }
    expect(f.cerrada).toBe(false)
    expect(consultas).toBe(0)
  })

  test('3 fallos seguidos → cierra SSE y consulta cada 3 s hasta que termina', async () => {
    respuestas = [
      { id: 7, estado: 'pendiente', posicion: 1, estimadoSegundos: 7 },
      new ApiError(0, 'Sin conexión'),
      { id: 7, estado: 'procesando' },
      { id: 7, estado: 'completado', resultado },
    ]
    seguir()
    const f = fuentes[0]
    f.cortar()
    f.cortar()
    f.cortar()
    expect(f.cerrada).toBe(true)
    expect(acciones.at(-1)).toEqual({ tipo: 'conexion', conexion: 'sondeo' })
    await esperar()
    expect(consultas).toBe(1)
    expect(acciones.at(-1)).toEqual({ tipo: 'estado', estado: { id: 7, estado: 'pendiente', posicion: 1, estimadoSegundos: 7 } })
    await r.avanzar() // fallo de red: sigue sondeando
    await esperar()
    expect(consultas).toBe(2)
    expect(r.cuantos).toBe(1)
    await r.avanzar()
    await esperar()
    await r.avanzar()
    await esperar()
    expect(consultas).toBe(4)
    expect(acciones.at(-1)).toEqual({ tipo: 'estado', estado: { id: 7, estado: 'completado', resultado } })
    expect(r.cuantos).toBe(0)
    expect(fuentes).toHaveLength(1) // no vuelve a abrir SSE
  })

  test('si el navegador da la conexión por cerrada, pasa a sondeo enseguida', async () => {
    respuestas = [{ id: 7, estado: 'cancelado' }]
    seguir()
    fuentes[0].cortar(true)
    await esperar()
    expect(consultas).toBe(1)
    expect(acciones.at(-1)).toEqual({ tipo: 'estado', estado: { id: 7, estado: 'cancelado' } })
    expect(r.cuantos).toBe(0)
  })

  test('404 durante el sondeo → consultaFallida y para', async () => {
    respuestas = [new ApiError(404, 'No existe')]
    seguir()
    fuentes[0].cortar(true)
    await esperar()
    expect(acciones.at(-1)).toEqual({ tipo: 'consultaFallida', status: 404, mensaje: 'No existe' })
    expect(r.cuantos).toBe(0)
  })

  test('detener durante el sondeo cancela el temporizador', async () => {
    const detener = seguir()
    fuentes[0].cortar(true)
    await esperar()
    expect(r.cuantos).toBe(1)
    detener()
    expect(r.cuantos).toBe(0)
  })
})

describe('casos límite', () => {
  test('un evento sin data se lee como objeto vacío', () => {
    seguir()
    fuentes[0].emitir('procesando', undefined)
    const e = new MessageEvent('procesando', { data: '' })
    ;(fuentes[0] as unknown as { oyentes: Map<string, ((e: Event) => void)[]> }).oyentes.get('procesando')?.forEach((h) => h(e))
    expect(acciones.at(-1)).toEqual({ tipo: 'evento', evento: { nombre: 'procesando', datos: {} } })
  })

  test('open y cortes después de detener se ignoran', () => {
    const detener = seguir()
    detener()
    fuentes[0].abrir()
    fuentes[0].cortar(true)
    expect(tipos()).toEqual(['conexion:conectando'])
    expect(consultas).toBe(0)
  })

  test('un error que no es de la API también se reintenta', async () => {
    respuestas = [new Error('raro'), new ApiError(500, 'caído'), { id: 7, estado: 'error', error: 'falló' }]
    seguir()
    fuentes[0].cortar(true)
    await esperar()
    expect(r.cuantos).toBe(1)
    await r.avanzar()
    await esperar()
    expect(r.cuantos).toBe(1)
    await r.avanzar()
    await esperar()
    expect(consultas).toBe(3)
    expect(acciones.at(-1)).toEqual({ tipo: 'estado', estado: { id: 7, estado: 'error', error: 'falló' } })
    expect(r.cuantos).toBe(0)
  })

  test('una consulta que vuelve después de detener no despacha', async () => {
    let resolver!: (e: EstadoSimulacion) => void
    let rechazar!: (e: Error) => void
    const despachadas: Accion[] = []
    const fuente = new FuenteFalsa()
    let n = 0
    const detener = seguirSimulacion(1, (a) => despachadas.push(a), {
      abrirEventos: () => fuente,
      consultar: () =>
        new Promise((res, rej) => {
          n++
          resolver = res
          rechazar = rej
        }),
      programar: r.programar,
      desprogramar: r.desprogramar,
    })
    fuente.cortar(true)
    detener()
    const antes = despachadas.length
    resolver({ id: 1, estado: 'procesando' })
    rechazar(new ApiError(404, 'x'))
    await esperar()
    expect(n).toBe(1)
    expect(despachadas).toHaveLength(antes)
    expect(r.cuantos).toBe(0)
  })

  test('sin programar/desprogramar usa setTimeout de verdad', async () => {
    const despachadas: Accion[] = []
    const fuente = new FuenteFalsa()
    const estados: EstadoSimulacion[] = [{ id: 1, estado: 'procesando' }, { id: 1, estado: 'cancelado' }]
    seguirSimulacion(
      1,
      (a) => despachadas.push(a),
      { abrirEventos: () => fuente, consultar: async () => estados.shift()! },
      { intervaloSondeoMs: 5, maxFallos: 1 },
    )
    fuente.cortar()
    await new Promise((res) => setTimeout(res, 40))
    expect(despachadas.at(-1)).toEqual({ tipo: 'estado', estado: { id: 1, estado: 'cancelado' } })
  })

  test('detener con el temporizador real lo cancela', async () => {
    let consultasReales = 0
    const fuente = new FuenteFalsa()
    const detener = seguirSimulacion(
      1,
      () => {},
      {
        abrirEventos: () => fuente,
        consultar: async () => {
          consultasReales++
          return { id: 1, estado: 'procesando' }
        },
      },
      { intervaloSondeoMs: 5, maxFallos: 1 },
    )
    fuente.cortar()
    await esperar()
    detener()
    await new Promise((res) => setTimeout(res, 30))
    expect(consultasReales).toBe(1)
  })
})
