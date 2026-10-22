import { describe, expect, test } from 'bun:test'
import { camposDeGen, MAX_MOVIMIENTOS, nuevoMiembro, nuevoUid, numeroGen, sanearMiembro } from './equipo.ts'

test('numeroGen', () => {
  expect(numeroGen('gen1')).toBe(1)
  expect(numeroGen('gen9')).toBe(9)
})

describe('camposDeGen', () => {
  test('gen 1: sin habilidad, naturaleza, objeto ni spd', () => {
    expect(camposDeGen('gen1')).toEqual({
      habilidad: false,
      naturaleza: false,
      objeto: false,
      stats: ['hp', 'atk', 'def', 'spa', 'spe'],
    })
  })

  test('gen 2: objeto y spa/spd separados', () => {
    const c = camposDeGen('gen2')
    expect(c.objeto).toBe(true)
    expect(c.habilidad).toBe(false)
    expect(c.naturaleza).toBe(false)
    expect(c.stats).toContain('spd')
  })

  test('desde gen 3: todo', () => {
    for (const gen of ['gen3', 'gen4', 'gen9'] as const) {
      expect(camposDeGen(gen)).toMatchObject({ habilidad: true, naturaleza: true, objeto: true })
      expect(camposDeGen(gen).stats).toHaveLength(6)
    }
  })
})

test('nuevoUid no se repite', () => {
  const uids = new Set(Array.from({ length: 50 }, nuevoUid))
  expect(uids.size).toBe(50)
})

test('nuevoMiembro empieza vacío', () => {
  const m = nuevoMiembro({ especie: 'Monferno', nombre: 'Monferno', tipos: ['Fire', 'Fighting'] }, '20')
  expect(m).toMatchObject({
    especie: 'Monferno',
    nombre: 'Monferno',
    tipos: ['Fire', 'Fighting'],
    nivel: '20',
    movimientos: [],
    habilidad: '',
    objeto: '',
    naturaleza: '',
  })
  expect(m.evs).toEqual({ hp: '', atk: '', def: '', spa: '', spd: '', spe: '' })
  expect(m.ivs).toEqual(m.evs)
  expect(m.evs).not.toBe(m.ivs)
  expect(nuevoMiembro({ especie: 'A', nombre: 'A', tipos: [] }).nivel).toBe('')
})

describe('sanearMiembro', () => {
  test('rechaza lo que no es objeto o no tiene especie', () => {
    for (const raw of [null, undefined, 3, 'Monferno', [], {}, { especie: '' }, { especie: 5 }]) {
      expect(sanearMiembro(raw)).toBeNull()
    }
  })

  test('rellena lo que falta y usa la especie como nombre', () => {
    const m = sanearMiembro({ especie: 'Grotle' })!
    expect(m.nombre).toBe('Grotle')
    expect(m.tipos).toEqual([])
    expect(m.movimientos).toEqual([])
    expect(m.evs.hp).toBe('')
    expect(m.uid).toBeTruthy()
  })

  test('descarta valores con otro tipo y recorta los movimientos', () => {
    const m = sanearMiembro({
      uid: 'viejo',
      especie: 'Grotle',
      nombre: 7,
      tipos: ['Grass', 3, null],
      nivel: 18,
      movimientos: ['A', 'B', 2, 'C', 'D', 'E'],
      habilidad: 'Overgrow',
      objeto: null,
      naturaleza: 'Adamant',
      evs: { hp: '4', atk: 252, spe: '252', nada: '1' },
      ivs: 'mal',
    })!
    expect(m.uid).not.toBe('viejo')
    expect(m.nombre).toBe('Grotle')
    expect(m.tipos).toEqual(['Grass'])
    expect(m.nivel).toBe('')
    expect(m.movimientos).toEqual(['A', 'B', 'C', 'D'])
    expect(m.movimientos).toHaveLength(MAX_MOVIMIENTOS)
    expect(m.habilidad).toBe('Overgrow')
    expect(m.objeto).toBe('')
    expect(m.naturaleza).toBe('Adamant')
    expect(m.evs).toEqual({ hp: '4', atk: '', def: '', spa: '', spd: '', spe: '252' })
    expect(m.ivs).toEqual({ hp: '', atk: '', def: '', spa: '', spd: '', spe: '' })
  })
})
