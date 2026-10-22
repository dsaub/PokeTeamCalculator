import { describe, expect, test } from 'bun:test'
import { nuevoMiembro, type MiembroEquipo } from './equipo.ts'
import { totalEvs, validarEquipo, validarMiembro } from './validacion.ts'

function miembro(cambios: Partial<MiembroEquipo> = {}): MiembroEquipo {
  return { ...nuevoMiembro({ especie: 'Monferno', nombre: 'Monferno', tipos: ['Fire', 'Fighting'] }), movimientos: ['Ember'], ...cambios }
}

const stats = (v: Partial<Record<string, string>>) => ({ hp: '', atk: '', def: '', spa: '', spd: '', spe: '', ...v })

describe('validarMiembro', () => {
  test('un miembro mínimo es válido', () => {
    expect(validarMiembro('gen4', miembro())).toEqual({})
  })

  test('nivel: vacío vale; fuera de 1–100 o no entero, no', () => {
    expect(validarMiembro('gen4', miembro({ nivel: '' })).nivel).toBeUndefined()
    expect(validarMiembro('gen4', miembro({ nivel: '100' })).nivel).toBeUndefined()
    expect(validarMiembro('gen4', miembro({ nivel: '0' })).nivel).toBe('Escribe un número entero entre 1 y 100.')
    expect(validarMiembro('gen4', miembro({ nivel: '101' })).nivel).toBeDefined()
    expect(validarMiembro('gen4', miembro({ nivel: '1.5' })).nivel).toBe('Escribe un número entero entre 1 y 100.')
    expect(validarMiembro('gen4', miembro({ nivel: '-3' })).nivel).toBeDefined()
  })

  test('movimientos: de 1 a 4 y sin repetir', () => {
    expect(validarMiembro('gen4', miembro({ movimientos: [] })).movimientos).toBe('Elige al menos un movimiento.')
    expect(validarMiembro('gen4', miembro({ movimientos: ['A', 'B', 'C', 'D'] })).movimientos).toBeUndefined()
    expect(validarMiembro('gen4', miembro({ movimientos: ['A', 'B', 'C', 'D', 'E'] })).movimientos).toBeDefined()
    expect(validarMiembro('gen4', miembro({ movimientos: ['A', 'A'] })).movimientos).toBe('Hay movimientos repetidos.')
  })

  test('movimientos que no puede aprender (equipo recuperado)', () => {
    const errores = validarMiembro('gen4', miembro({ movimientos: ['Ember', 'Surf'] }), new Set(['Ember']))
    expect(errores.movimientos).toBe('Algún movimiento no lo puede aprender en este juego.')
  })

  test('EVs: 0–252 por stat y 510 en total', () => {
    expect(validarMiembro('gen4', miembro({ evs: stats({ atk: '252', spe: '252', hp: '6' }) }))).toEqual({})
    expect(validarMiembro('gen4', miembro({ evs: stats({ atk: '253' }) }))['evs.atk']).toBe('Escribe un número entero entre 0 y 252.')
    const total = validarMiembro('gen4', miembro({ evs: stats({ atk: '252', spe: '252', hp: '8' }) }))
    expect(total.evsTotal).toBe('Los EVs suman 512; el máximo es 510.')
  })

  test('IVs: 0–31; el 0 es válido', () => {
    expect(validarMiembro('gen4', miembro({ ivs: stats({ atk: '0' }) }))).toEqual({})
    expect(validarMiembro('gen4', miembro({ ivs: stats({ atk: '32' }) }))['ivs.atk']).toBe('Escribe un número entero entre 0 y 31.')
    expect(validarMiembro('gen4', miembro({ ivs: stats({ atk: 'x' }) }))['ivs.atk']).toBeDefined()
  })

  test('en gen 1 se ignora spd; desde gen 2 se valida', () => {
    expect(validarMiembro('gen1', miembro({ ivs: stats({ spd: '99' }) }))).toEqual({})
    expect(validarMiembro('gen2', miembro({ ivs: stats({ spd: '99' }) }))['ivs.spd']).toBeDefined()
    expect(validarMiembro('gen3', miembro({ ivs: stats({ spd: '99' }) }))['ivs.spd']).toBeDefined()
  })
})

describe('validarEquipo', () => {
  test('equipo vacío o de más de 6 no es válido', () => {
    expect(validarEquipo('gen4', []).valido).toBe(false)
    expect(validarEquipo('gen4', []).general).toBe('Añade al menos un Pokémon a tu equipo.')
    expect(validarEquipo('gen4', Array.from({ length: 7 }, () => miembro())).valido).toBe(false)
  })

  test('es válido si todos los miembros lo son', () => {
    expect(validarEquipo('gen4', [miembro(), miembro()]).valido).toBe(true)
    const r = validarEquipo('gen4', [miembro(), miembro({ movimientos: [] })])
    expect(r.valido).toBe(false)
    expect(r.porMiembro[0]).toEqual({})
    expect(r.porMiembro[1].movimientos).toBeDefined()
  })
})

describe('totalEvs', () => {
  test('suma solo los enteros válidos', () => {
    expect(totalEvs('gen4', stats({ hp: '252', atk: ' 4 ', spe: 'x', def: '-3', spa: '1.5' }))).toBe(256)
  })

  test('en gen 1 no cuenta spd', () => {
    expect(totalEvs('gen1', stats({ spa: '100', spd: '100' }))).toBe(100)
    expect(totalEvs('gen2', stats({ spa: '100', spd: '100' }))).toBe(200)
  })

  test('vacío → 0', () => {
    expect(totalEvs('gen9', stats({}))).toBe(0)
  })
})
