import { describe, expect, test } from 'bun:test'
import type { ResultadoPokemon } from '../api/tipos.ts'
import { decimal, empatados, explicacionEmpate, fraseRecomendacion, leerRival, mensajeDeError, porcentaje, textoPosicion, tiempoEstimado, turnos } from './textos.ts'

const r = (c: Partial<ResultadoPokemon>): ResultadoPokemon => ({
  pokemon: 'Grotle',
  nivel: 18,
  victorias: 300,
  derrotas: 0,
  empates: 0,
  winrate: 1,
  psMedioVictoria: 0.754,
  turnosMedio: 1.4,
  ...c,
})

const nombre = (e: string) => ({ Grotle: 'Grotle', Monferno: 'Monferno' })[e] ?? e

test('porcentaje en español', () => {
  expect(porcentaje(0.754)).toMatch(/^75\s%$/)
  expect(porcentaje(1)).toMatch(/^100\s%$/)
})

test('tiempo estimado sin falsa precisión', () => {
  expect(tiempoEstimado(3)).toBe('unos segundos')
  expect(tiempoEstimado(20)).toBe('unos 20 segundos')
  expect(tiempoEstimado(14)).toBe('unos 15 segundos')
  expect(tiempoEstimado(70)).toBe('alrededor de un minuto')
  expect(tiempoEstimado(300)).toBe('unos 5 minutos')
})

describe('fraseRecomendacion', () => {
  test('explica con winrate, PS y turnos', () => {
    expect(fraseRecomendacion(r({ winrate: 0.897, psMedioVictoria: 0.415, turnosMedio: 3.08 }))).toMatch(
      /^Gana el 90\s% de los combates y termina con el 42\s% de sus PS de media\. Cada combate dura 3,1 turnos\.$/,
    )
    expect(fraseRecomendacion(r({}))).toStartWith('Gana todos los combates')
  })

  test('avisa si ni el mejor gana la mitad', () => {
    expect(fraseRecomendacion(r({ winrate: 0.3, victorias: 90 }))).toContain('pierde más de lo que gana')
    expect(fraseRecomendacion(r({ winrate: 0, victorias: 0 }))).toStartWith('Ninguno de tus Pokémon gana')
  })
})

describe('explicacionEmpate', () => {
  test('null si no hay empate', () => {
    expect(explicacionEmpate(r({}), r({ pokemon: 'Monferno', winrate: 0.9 }), nombre)).toBeNull()
  })

  test('por PS', () => {
    const t = explicacionEmpate(r({}), r({ pokemon: 'Monferno', psMedioVictoria: 0.6 }), nombre)
    expect(t).toMatch(/^Grotle y Monferno ganan el 100\s% de los combates, pero a Grotle le quedan más PS al ganar/)
  })

  test('por turnos si los PS son iguales', () => {
    const t = explicacionEmpate(r({}), r({ pokemon: 'Monferno', turnosMedio: 1.93 }), nombre)
    expect(t).toContain('pero Grotle acaba antes (1,4 turnos frente a 1,9 turnos)')
  })

  test('sin diferencias visibles', () => {
    expect(explicacionEmpate(r({}), r({ pokemon: 'Monferno' }), nombre)).toContain('cualquiera de los dos')
  })
})

test('leerRival', () => {
  expect(leerRival('Cranidos (Nv. 14)')).toEqual({ especie: 'Cranidos', nivel: 14 })
  expect(leerRival('Mr. Mime (Nv. 5)')).toEqual({ especie: 'Mr. Mime', nivel: 5 })
  expect(leerRival('raro')).toBeNull()
})

test('decimal: como mucho un decimal y coma', () => {
  expect(decimal(1.44)).toBe('1,4')
  expect(decimal(2)).toBe('2')
  expect(decimal(0.05)).toBe('0,1')
})

test('turnos: singular y plural', () => {
  expect(turnos(1)).toBe('1 turno')
  expect(turnos(2)).toBe('2 turnos')
  expect(turnos(1.4)).toBe('1,4 turnos')
})

test('textoPosicion', () => {
  expect(textoPosicion(1)).toBe('Eres el siguiente en la cola')
  expect(textoPosicion(0)).toBe('Eres el siguiente en la cola')
  expect(textoPosicion(2)).toBe('Hay 1 por delante en la cola')
  expect(textoPosicion(10)).toBe('Hay 9 por delante en la cola')
})

describe('tiempoEstimado: límites', () => {
  const casos: [number, string][] = [
    [Number.NaN, 'un momento'],
    [Number.POSITIVE_INFINITY, 'un momento'],
    [-1, 'un momento'],
    [0, 'unos segundos'],
    [7.9, 'unos segundos'],
    [8, 'unos 10 segundos'],
    [23, 'unos 25 segundos'],
    [54, 'unos 55 segundos'],
    [55, 'alrededor de un minuto'],
    [89, 'alrededor de un minuto'],
    [90, 'unos 2 minutos'],
    [600, 'unos 10 minutos'],
  ]
  for (const [s, texto] of casos) test(`${s} s → ${texto}`, () => expect(tiempoEstimado(s)).toBe(texto))
})

test('empatados compara al redondeo mostrado', () => {
  expect(empatados(r({ winrate: 0.501 }), r({ winrate: 0.499 }))).toBe(true)
  expect(empatados(r({ winrate: 0.51 }), r({ winrate: 0.49 }))).toBe(false)
})

test('fraseRecomendacion: sin victorias', () => {
  expect(fraseRecomendacion(r({ victorias: 0, winrate: 0 }))).toContain('Ninguno de tus Pokémon gana')
})

test('fraseRecomendacion: gana el 100 % sin aviso', () => {
  const f = fraseRecomendacion(r({ winrate: 1, turnosMedio: 1 }))
  expect(f).toStartWith('Gana todos los combates')
  expect(f).toContain('1 turno.')
  expect(f).not.toContain('Aun así')
})

test('leerRival: formatos que no encajan → null', () => {
  expect(leerRival('Cranidos')).toBeNull()
  expect(leerRival('Cranidos (Nv. x)')).toBeNull()
  expect(leerRival('Mr. Mime (Nv. 30)')).toEqual({ especie: 'Mr. Mime', nivel: 30 })
})

test('mensajeDeError', () => {
  expect(mensajeDeError(new Error('falló'))).toBe('falló')
  expect(mensajeDeError('texto suelto')).toBe('Ha ocurrido un error inesperado.')
  expect(mensajeDeError(undefined)).toBe('Ha ocurrido un error inesperado.')
})
