import { describe, expect, test } from 'bun:test'
import type { Resultado } from '../api/tipos.ts'
import { ESTADO_INICIAL, reducir, type Accion, type EstadoMaquina } from './maquina.ts'

const resultado: Resultado = { gen: 'gen4', rival: 'Cranidos (Nv. 14)', simulaciones: 300, resultados: [], ms: 1240 }

const aplicar = (acciones: Accion[], desde: EstadoMaquina = ESTADO_INICIAL) => acciones.reduce(reducir, desde)

const pendiente = (posicion: number): Accion => ({
  tipo: 'evento',
  evento: { nombre: 'posicion', datos: { posicion, estimadoSegundos: posicion * 7 } },
})

describe('creación', () => {
  test('202 → pendiente con la posición y el aviso de worker', () => {
    const s = aplicar([
      { tipo: 'creada', respuesta: { id: 1, estado: 'pendiente', posicion: 3, estimadoSegundos: 20, workerDisponible: false } },
    ])
    expect(s.fase).toEqual({ tipo: 'pendiente', posicion: 3, estimadoSegundos: 20 })
    expect(s.workerDisponible).toBe(false)
  })

  test('200 de caché → completado al instante, marcado como caché', () => {
    const s = aplicar([{ tipo: 'creada', respuesta: { id: 1, estado: 'completado', cache: true, resultado } }])
    expect(s.fase).toEqual({ tipo: 'completado', resultado, cache: true })
  })
})

describe('recorrido normal', () => {
  test('cargando → pendiente 3 → 2 → 1 → procesando → completado', () => {
    let s = reducir(ESTADO_INICIAL, { tipo: 'estado', estado: { id: 1, estado: 'pendiente', posicion: 3, estimadoSegundos: 21 } })
    expect(s.fase).toMatchObject({ tipo: 'pendiente', posicion: 3 })
    s = aplicar([{ tipo: 'conexion', conexion: 'conectado' }, pendiente(2), pendiente(1)], s)
    expect(s.fase).toMatchObject({ tipo: 'pendiente', posicion: 1 })
    expect(s.conexion).toBe('conectado')
    s = reducir(s, { tipo: 'evento', evento: { nombre: 'procesando', datos: {} } })
    expect(s.fase).toEqual({ tipo: 'procesando' })
    s = reducir(s, { tipo: 'evento', evento: { nombre: 'completado', datos: { resultado } } })
    expect(s.fase).toEqual({ tipo: 'completado', resultado, cache: false })
    expect(s.conexion).toBe('inactiva')
  })

  test('GET de cada estado final', () => {
    expect(reducir(ESTADO_INICIAL, { tipo: 'estado', estado: { id: 1, estado: 'error', error: 'Falló' } }).fase).toEqual({
      tipo: 'error',
      mensaje: 'Falló',
    })
    expect(reducir(ESTADO_INICIAL, { tipo: 'estado', estado: { id: 1, estado: 'cancelado' } }).fase).toEqual({
      tipo: 'cancelado',
      porUsuario: false,
    })
    expect(reducir(ESTADO_INICIAL, { tipo: 'estado', estado: { id: 1, estado: 'procesando' } }).fase).toEqual({ tipo: 'procesando' })
  })
})

describe('errores', () => {
  const enCola = aplicar([{ tipo: 'estado', estado: { id: 1, estado: 'pendiente', posicion: 2, estimadoSegundos: 14 } }])

  test('evento error → error con el mensaje de la API', () => {
    const s = reducir(enCola, { tipo: 'evento', evento: { nombre: 'error', datos: { error: 'Candidato 1: no existe' } } })
    expect(s.fase).toEqual({ tipo: 'error', mensaje: 'Candidato 1: no existe' })
  })

  test('cancelado por el servidor', () => {
    const s = reducir(enCola, { tipo: 'evento', evento: { nombre: 'cancelado', datos: {} } })
    expect(s.fase).toEqual({ tipo: 'cancelado', porUsuario: false })
  })

  test('404 al cargar → no encontrada', () => {
    expect(reducir(ESTADO_INICIAL, { tipo: 'consultaFallida', status: 404, mensaje: 'x' }).fase).toEqual({ tipo: 'noEncontrada' })
  })

  test('fallo de red al cargar → falloCarga, y reintentar vuelve a cargando', () => {
    const s = reducir(ESTADO_INICIAL, { tipo: 'consultaFallida', status: 0, mensaje: 'Sin conexión' })
    expect(s.fase).toEqual({ tipo: 'falloCarga', mensaje: 'Sin conexión' })
    expect(reducir(s, { tipo: 'reintentar' }).fase).toEqual({ tipo: 'cargando' })
  })

  test('fallo de red durante el seguimiento no cambia la fase', () => {
    expect(reducir(enCola, { tipo: 'consultaFallida', status: 0, mensaje: 'x' })).toBe(enCola)
  })
})

describe('cancelación por el jugador', () => {
  const enCola = aplicar([{ tipo: 'estado', estado: { id: 1, estado: 'pendiente', posicion: 2, estimadoSegundos: 14 } }])

  test('cancelando → cancelada', () => {
    const s = aplicar([{ tipo: 'cancelando' }, { tipo: 'cancelada' }], enCola)
    expect(s.fase).toEqual({ tipo: 'cancelado', porUsuario: true })
    expect(s.cancelacion).toEqual({ tipo: 'no' })
  })

  test('si el evento cancelado llega antes que el 204, también es del jugador', () => {
    const s = aplicar([{ tipo: 'cancelando' }, { tipo: 'evento', evento: { nombre: 'cancelado', datos: {} } }], enCola)
    expect(s.fase).toEqual({ tipo: 'cancelado', porUsuario: true })
  })

  test('fallo al cancelar se muestra y la simulación sigue', () => {
    const s = aplicar([{ tipo: 'cancelando' }, { tipo: 'cancelacionFallida', mensaje: 'Sin conexión' }], enCola)
    expect(s.fase.tipo).toBe('pendiente')
    expect(s.cancelacion).toEqual({ tipo: 'fallo', mensaje: 'Sin conexión' })
  })

  test('no se puede cancelar algo terminado', () => {
    const fin = reducir(enCola, { tipo: 'evento', evento: { nombre: 'completado', datos: { resultado } } })
    expect(reducir(fin, { tipo: 'cancelando' })).toBe(fin)
  })
})

describe('estados finales', () => {
  test('ignoran todo lo que llegue después', () => {
    const fin = reducir(ESTADO_INICIAL, { tipo: 'estado', estado: { id: 1, estado: 'completado', resultado } })
    for (const a of [
      pendiente(1),
      { tipo: 'estado', estado: { id: 1, estado: 'procesando' } },
      { tipo: 'conexion', conexion: 'sondeo' },
      { tipo: 'consultaFallida', status: 404, mensaje: 'x' },
      { tipo: 'cancelada' },
    ] satisfies Accion[]) {
      expect(reducir(fin, a)).toBe(fin)
    }
  })
})

test('conexión: reconectando y sondeo se reflejan mientras está en curso', () => {
  const s = aplicar([
    { tipo: 'estado', estado: { id: 1, estado: 'procesando' } },
    { tipo: 'conexion', conexion: 'reconectando' },
  ])
  expect(s.conexion).toBe('reconectando')
  expect(reducir(s, { tipo: 'conexion', conexion: 'sondeo' }).conexion).toBe('sondeo')
})
