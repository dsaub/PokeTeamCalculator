import { describe, expect, test } from 'bun:test'
import {
  cargarContexto,
  cargarEquipo,
  cargarUltimoJuego,
  guardarContexto,
  guardarEquipo,
  guardarUltimoJuego,
  type ContextoSimulacion,
} from './almacen.ts'
import { nuevoMiembro } from './equipo.ts'

function almacenEnMemoria(): Storage {
  const datos = new Map<string, string>()
  return {
    get length() {
      return datos.size
    },
    clear: () => datos.clear(),
    getItem: (k) => datos.get(k) ?? null,
    setItem: (k, v) => void datos.set(k, String(v)),
    removeItem: (k) => void datos.delete(k),
    key: (i) => [...datos.keys()][i] ?? null,
  }
}

const monferno = () => ({ ...nuevoMiembro({ especie: 'Monferno', nombre: 'Monferno', tipos: ['Fire'] }, '16'), movimientos: ['Ember'] })

describe('equipo guardado', () => {
  test('se guarda y se recupera por juego', () => {
    const a = almacenEnMemoria()
    expect(guardarEquipo('platino', [monferno()], () => a)).toBe(true)
    const recuperado = cargarEquipo('platino', () => a)
    expect(recuperado).toHaveLength(1)
    expect(recuperado[0]).toMatchObject({ especie: 'Monferno', nivel: '16', movimientos: ['Ember'] })
    expect(recuperado[0].uid).toBeString()
    expect(cargarEquipo('rojo', () => a)).toEqual([])
  })

  test('sin localStorage: no lanza, guarda false y carga vacío', () => {
    const lanza = () => {
      throw new DOMException('denegado', 'SecurityError')
    }
    expect(guardarEquipo('platino', [monferno()], lanza)).toBe(false)
    expect(cargarEquipo('platino', lanza)).toEqual([])
    expect(guardarEquipo('platino', [monferno()], () => null)).toBe(false)
  })

  test('almacén lleno: setItem lanza y se devuelve false', () => {
    const a = almacenEnMemoria()
    a.setItem = () => {
      throw new DOMException('lleno', 'QuotaExceededError')
    }
    expect(guardarEquipo('platino', [monferno()], () => a)).toBe(false)
  })

  test('datos corruptos o con otra forma: se descartan', () => {
    const a = almacenEnMemoria()
    a.setItem('contrataque:v2:equipo:platino', '{no es json')
    expect(cargarEquipo('platino', () => a)).toEqual([])
    a.setItem('contrataque:v2:equipo:platino', JSON.stringify([{ nombre: 'sin especie' }, { especie: 'Onix', nivel: 12, movimientos: 'x' }]))
    const r = cargarEquipo('platino', () => a)
    expect(r).toHaveLength(1)
    expect(r[0]).toMatchObject({ especie: 'Onix', nivel: '', movimientos: [] })
  })
})

describe('contexto de simulación', () => {
  const ctx: ContextoSimulacion = {
    juego: 'platino',
    lider: 'roark',
    rival: 0,
    peticion: { gen: 'gen4', rival: { especie: 'Cranidos', nivel: 14, movimientos: ['Leer'] }, candidatos: [] },
    nombres: { Cranidos: 'Cranidos' },
  }

  test('se guarda por id y se recupera', () => {
    const a = almacenEnMemoria()
    guardarContexto(5, ctx, () => a)
    guardarContexto(6, { ...ctx, rival: 1 }, () => a)
    expect(cargarContexto(5, () => a)?.rival).toBe(0)
    expect(cargarContexto(6, () => a)?.rival).toBe(1)
    expect(cargarContexto(7, () => a)).toBeNull()
  })

  test('guarda como mucho 20', () => {
    const a = almacenEnMemoria()
    for (let i = 1; i <= 25; i++) guardarContexto(i, ctx, () => a)
    expect(cargarContexto(25, () => a)).not.toBeNull()
    expect(cargarContexto(5, () => a)).toBeNull()
  })
})

describe('último juego', () => {
  test('se guarda y se recupera', () => {
    const a = almacenEnMemoria()
    expect(cargarUltimoJuego(() => a)).toBeNull()
    guardarUltimoJuego('platino', () => a)
    expect(cargarUltimoJuego(() => a)).toBe('platino')
  })

  test('un valor que no es texto se ignora', () => {
    const a = almacenEnMemoria()
    a.setItem('contrataque:v2:juego', '42')
    expect(cargarUltimoJuego(() => a)).toBeNull()
    a.setItem('contrataque:v2:juego', '{roto')
    expect(cargarUltimoJuego(() => a)).toBeNull()
  })

  test('si el acceso al almacén lanza, no rompe', () => {
    const lanza = () => {
      throw new Error('SecurityError')
    }
    expect(() => guardarUltimoJuego('platino', lanza)).not.toThrow()
    expect(cargarUltimoJuego(lanza)).toBeNull()
  })

  test('por defecto usa el localStorage del navegador', () => {
    guardarUltimoJuego('perla')
    expect(localStorage.getItem('contrataque:v2:juego')).toBe('"perla"')
    expect(cargarUltimoJuego()).toBe('perla')
    expect(guardarEquipo('perla', [monferno()])).toBe(true)
    expect(cargarEquipo('perla')).toHaveLength(1)
  })
})

describe('contexto: datos inválidos', () => {
  test('lista con entradas rotas o contexto incompleto → null', () => {
    const a = almacenEnMemoria()
    a.setItem(
      'contrataque:v2:simulaciones',
      JSON.stringify([7, [1, { juego: 3 }], [2, { juego: 'platino', nombres: {} }], [3, null]]),
    )
    expect(cargarContexto(1, () => a)).toBeNull()
    expect(cargarContexto(2, () => a)).toBeNull()
    expect(cargarContexto(3, () => a)).toBeNull()
    expect(cargarContexto(4, () => a)).toBeNull()
  })

  test('no es una lista → null, y guardar la reemplaza', () => {
    const a = almacenEnMemoria()
    a.setItem('contrataque:v2:simulaciones', '{"no":"lista"}')
    expect(cargarContexto(1, () => a)).toBeNull()
    const ctx = { juego: 'platino', lider: 'Roark', rival: 0, peticion: {}, nombres: {} } as unknown as ContextoSimulacion
    guardarContexto(1, ctx, () => a)
    expect(cargarContexto(1, () => a)).toEqual(ctx)
  })
})
