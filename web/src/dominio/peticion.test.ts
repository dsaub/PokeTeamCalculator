import { describe, expect, test } from 'bun:test'
import type { PokemonLider } from '../api/tipos.ts'
import { nuevoMiembro, type MiembroEquipo } from './equipo.ts'
import { construirCandidato, construirPeticion, construirRival } from './peticion.ts'

const cranidos: PokemonLider = {
  especie: 'Cranidos',
  nombre: 'Cranidos',
  nivel: 14,
  habilidad: 'Mold Breaker',
  objeto: null,
  movimientos: ['Headbutt', 'Pursuit', 'Leer'],
  tipos: ['Rock'],
}

function miembro(cambios: Partial<MiembroEquipo> = {}): MiembroEquipo {
  return { ...nuevoMiembro({ especie: 'Mr. Mime', nombre: 'Mr. Mime', tipos: ['Psychic'] }), movimientos: ['Confusion'], ...cambios }
}

describe('construirCandidato', () => {
  test('solo especie y movimientos si no se rellena nada más', () => {
    expect(construirCandidato('gen4', miembro())).toEqual({ especie: 'Mr. Mime', movimientos: ['Confusion'] })
  })

  test('usa la especie en formato Showdown, nunca el nombre para mostrar', () => {
    const m = miembro({ especie: 'Weezing-Galar', nombre: 'Weezing de Galar' })
    const c = construirCandidato('gen8', m)
    expect(c.especie).toBe('Weezing-Galar')
    expect(JSON.stringify(c)).not.toContain('Weezing de Galar')
    expect(c).not.toHaveProperty('nombre')
  })

  test('no envía campos vacíos ni con espacios', () => {
    const m = miembro({ nivel: '  ', objeto: '   ', habilidad: '', naturaleza: '' })
    const c = construirCandidato('gen4', m)
    for (const campo of ['nivel', 'objeto', 'habilidad', 'naturaleza', 'evs', 'ivs']) expect(c).not.toHaveProperty(campo)
  })

  test('envía los opcionales rellenos como números', () => {
    const m = miembro({
      nivel: '16',
      habilidad: 'Filter',
      objeto: ' Oran Berry ',
      naturaleza: 'Modest',
      evs: { hp: '', atk: '', def: '', spa: '252', spd: '', spe: '4' },
      ivs: { hp: '31', atk: '', def: '', spa: '', spd: '', spe: '' },
    })
    expect(construirCandidato('gen4', m)).toEqual({
      especie: 'Mr. Mime',
      movimientos: ['Confusion'],
      nivel: 16,
      habilidad: 'Filter',
      objeto: 'Oran Berry',
      naturaleza: 'Modest',
      evs: { spa: 252, spe: 4 },
      ivs: { hp: 31 },
    })
  })

  test('un IV a 0 sí se envía', () => {
    const m = miembro({ ivs: { hp: '', atk: '0', def: '', spa: '', spd: '', spe: '' } })
    expect(construirCandidato('gen4', m).ivs).toEqual({ atk: 0 })
  })

  test('gen 2: sin habilidad ni naturaleza, con objeto y con spa/spd', () => {
    const m = miembro({
      habilidad: 'Filter',
      naturaleza: 'Modest',
      objeto: 'Leftovers',
      evs: { hp: '', atk: '', def: '', spa: '100', spd: '100', spe: '' },
    })
    const c = construirCandidato('gen2', m)
    expect(c).not.toHaveProperty('habilidad')
    expect(c).not.toHaveProperty('naturaleza')
    expect(c.objeto).toBe('Leftovers')
    expect(c.evs).toEqual({ spa: 100, spd: 100 })
  })

  test('gen 1: tampoco objeto', () => {
    const m = miembro({ habilidad: 'Filter', naturaleza: 'Modest', objeto: 'Leftovers' })
    expect(construirCandidato('gen1', m)).toEqual({ especie: 'Mr. Mime', movimientos: ['Confusion'] })
  })
})

describe('construirRival', () => {
  test('copia el Pokémon del líder sin los campos de visualización ni nulos', () => {
    expect(construirRival('gen4', cranidos)).toEqual({
      especie: 'Cranidos',
      nivel: 14,
      habilidad: 'Mold Breaker',
      movimientos: ['Headbutt', 'Pursuit', 'Leer'],
    })
  })

  test('aplica las reglas de generación también al rival', () => {
    const r = construirRival('gen1', { ...cranidos, objeto: 'Leftovers' })
    expect(r).not.toHaveProperty('habilidad')
    expect(r).not.toHaveProperty('objeto')
  })
})

test('construirPeticion no incluye número de simulaciones ni opciones', () => {
  const p = construirPeticion('gen4', cranidos, [miembro(), miembro({ especie: 'Monferno', nombre: 'Monferno' })])
  expect(Object.keys(p).sort()).toEqual(['candidatos', 'gen', 'rival'])
  expect(p.candidatos.map((c) => c.especie)).toEqual(['Mr. Mime', 'Monferno'])
})
