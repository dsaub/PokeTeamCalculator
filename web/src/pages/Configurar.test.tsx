import { beforeEach, describe, expect, test } from 'bun:test'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useParams } from 'react-router'
import { json, rutas, type Llamada } from '../../test/fetch.ts'
import { vaciarCacheCatalogo } from '../api/catalogo.ts'
import type { Juego, Lider, PokemonDetalle, PokemonResumen } from '../api/tipos.ts'
import { cargarContexto, cargarEquipo, guardarEquipo, guardarUltimoJuego } from '../dominio/almacen.ts'
import { nuevoMiembro } from '../dominio/equipo.ts'
import { Configurar } from './Configurar.tsx'

const JUEGOS: Juego[] = [
  { id: 'platino', nombre: 'Platino', gen: 'gen4' },
  { id: 'rojo', nombre: 'Rojo', gen: 'gen1' },
]

const lider = (id: number, nombre: string, equipo: [string, number][]): Lider => ({
  id,
  nombre,
  ciudad: `Ciudad ${nombre}`,
  tipo: 'rock',
  orden: id,
  equipo: equipo.map(([especie, nivel]) => ({
    especie,
    nombre: especie,
    nivel,
    habilidad: null,
    objeto: null,
    movimientos: ['Headbutt'],
    tipos: ['rock'],
  })),
})

const LIDERES_PLATINO = [
  lider(1, 'Roark', [
    ['Geodude', 12],
    ['Onix', 12],
    ['Cranidos', 14],
  ]),
  lider(2, 'Gardenia', [['Turtwig', 20]]),
]
const LIDERES_ROJO = [lider(3, 'Brock', [['Onix', 14]])]

const BUSQUEDA: PokemonResumen[] = [{ especie: 'Monferno', nombre: 'Monferno', tipos: ['fire', 'fighting'] }]

const DETALLE: PokemonDetalle = {
  ...BUSQUEDA[0]!,
  habilidades: [
    { id: 'Blaze', nombre: 'Mar Llamas' },
    { id: 'Iron Fist', nombre: 'Puño Férreo' },
  ],
  movimientos: ['Mach Punch', 'Ember', 'Scratch', 'Leer', 'Taunt'].map((id) => ({
    id,
    nombre: `${id} ES`,
    tipo: 'Normal',
    categoria: 'physical' as const,
    potencia: 40,
  })),
}

let llamadas: Llamada[]

function apiNormal(extra: Record<string, unknown> = {}) {
  llamadas = rutas({
    'GET /juegos': JUEGOS,
    'GET /juegos/platino/lideres': LIDERES_PLATINO,
    'GET /juegos/rojo/lideres': LIDERES_ROJO,
    'GET /pokemon': BUSQUEDA,
    'GET /pokemon/Monferno': DETALLE,
    ...extra,
  })
}

function Destino() {
  return <p>destino {useParams().id}</p>
}

function abrir() {
  render(
    <MemoryRouter>
      <Routes>
        <Route path="/" element={<Configurar />} />
        <Route path="/simulacion/:id" element={<Destino />} />
      </Routes>
    </MemoryRouter>,
  )
}

const boton = (nombre: string | RegExp) => screen.getByRole('button', { name: nombre })
const pulsado = (nombre: string | RegExp) => boton(nombre).getAttribute('aria-pressed')
const alerta = () => screen.findByRole('alert')

async function añadirMonferno(movs = ['Mach Punch ES']) {
  const u = userEvent.setup()
  await u.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
  await u.click(boton('Buscar'))
  await u.click(await screen.findByRole('button', { name: /Monferno/ }))
  await screen.findByLabelText('Nivel')
  for (const m of movs) await u.click(boton(new RegExp(`^${m}`)))
  await u.click(boton('Añadir al equipo'))
}

beforeEach(() => vaciarCacheCatalogo())

describe('carga inicial', () => {
  test('elige el primer juego y el último Pokémon del primer líder', async () => {
    apiNormal()
    abrir()
    expect(await screen.findByRole('button', { name: /Roark/ })).toBeTruthy()
    expect(pulsado(/Platino/)).toBe('true')
    expect(pulsado(/Roark/)).toBe('true')
    expect(pulsado('Cranidos · Nv. 14')).toBe('true')
    expect(pulsado('Geodude · Nv. 12')).toBe('false')
    expect(screen.getByText('0 / 6')).toBeTruthy()
    expect(screen.getByText('Busca un Pokémon y añádelo con sus movimientos.')).toBeTruthy()
  })

  test('recupera el último juego y su equipo guardado', async () => {
    guardarUltimoJuego('rojo')
    guardarEquipo('rojo', [{ ...nuevoMiembro({ especie: 'Pikachu', nombre: 'Pikachu', tipos: ['electric'] }), movimientos: ['Thunder Shock'] }])
    apiNormal()
    abrir()
    expect(await screen.findByRole('button', { name: /Brock/ })).toBeTruthy()
    expect(pulsado(/Rojo/)).toBe('true')
    expect(screen.getByText('Pikachu')).toBeTruthy()
    expect(screen.getByText('1 / 6')).toBeTruthy()
  })

  test('sin juegos → aviso', async () => {
    rutas({ 'GET /juegos': [] })
    abrir()
    expect((await alerta()).textContent).toBe('No hay juegos con gimnasios disponibles.')
  })

  test('error al cargar juegos → mensaje de la API', async () => {
    rutas({ 'GET /juegos': json({ error: 'Catálogo no disponible.' }, 500) })
    abrir()
    expect((await alerta()).textContent).toBe('Catálogo no disponible.')
  })

  test('error al cargar líderes → mensaje', async () => {
    rutas({ 'GET /juegos': JUEGOS, 'GET /juegos/platino/lideres': json({ error: 'Sin líderes.' }, 500) })
    abrir()
    expect((await alerta()).textContent).toBe('Sin líderes.')
  })

  test('juego sin líderes', async () => {
    rutas({ 'GET /juegos': JUEGOS, 'GET /juegos/platino/lideres': [] })
    abrir()
    expect(await screen.findByText('Este juego no tiene líderes de gimnasio.')).toBeTruthy()
  })
})

describe('líder y rival', () => {
  test('cambiar de juego carga sus líderes y lo recuerda', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await userEvent.click(boton(/Rojo/))
    expect(await screen.findByRole('button', { name: /Brock/ })).toBeTruthy()
    expect(localStorage.getItem('contrataque:v2:juego')).toBe('"rojo"')
  })

  test('cambiar de líder elige su último Pokémon; se puede elegir otro rival', async () => {
    apiNormal()
    abrir()
    await userEvent.click(await screen.findByRole('button', { name: /Gardenia/ }))
    expect(pulsado(/Gardenia/)).toBe('true')
    expect(pulsado('Turtwig · Nv. 20')).toBe('true')
    await userEvent.click(boton(/Roark/))
    await userEvent.click(boton('Onix · Nv. 12'))
    expect(pulsado('Onix · Nv. 12')).toBe('true')
    expect(pulsado('Cranidos · Nv. 14')).toBe('false')
  })
})

describe('equipo', () => {
  test('búsqueda de menos de dos caracteres → error sin llamar a la API', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await userEvent.type(screen.getByLabelText('Buscar Pokémon'), ' m ')
    await userEvent.click(boton('Buscar'))
    expect((await alerta()).textContent).toBe('Escribe al menos dos caracteres para buscar.')
    expect(llamadas.some((l) => l.url.includes('/pokemon'))).toBe(false)
  })

  test('error al buscar o al pedir el detalle', async () => {
    apiNormal({ 'GET /pokemon': json({ error: 'Búsqueda caída.' }, 500) })
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await userEvent.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
    await userEvent.click(boton('Buscar'))
    expect((await alerta()).textContent).toBe('Búsqueda caída.')

    vaciarCacheCatalogo()
    apiNormal({ 'GET /pokemon/Monferno': json({ error: 'Detalle caído.' }, 500) })
    await userEvent.click(boton('Buscar'))
    await userEvent.click(await screen.findByRole('button', { name: /Monferno/ }))
    expect((await alerta()).textContent).toBe('Detalle caído.')
  })

  test('elige como mucho 4 movimientos y se pueden desmarcar', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await userEvent.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
    await userEvent.click(boton('Buscar'))
    await userEvent.click(await screen.findByRole('button', { name: /Monferno/ }))
    expect((screen.getByLabelText('Habilidad') as HTMLSelectElement).value).toBe('Blaze')
    for (const m of ['Mach Punch', 'Ember', 'Scratch', 'Leer', 'Taunt']) await userEvent.click(boton(new RegExp(`^${m} ES`)))
    expect(screen.getByText('Movimientos (4 / 4)')).toBeTruthy()
    expect(pulsado(/^Taunt ES/)).toBe('false')
    await userEvent.click(boton(/^Ember ES/))
    expect(screen.getByText('Movimientos (3 / 4)')).toBeTruthy()
  })

  test('añadir sin movimientos → error', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await añadirMonferno([])
    expect((await alerta()).textContent).toBe('Elige al menos un movimiento.')
  })

  test('cancelar la selección la cierra', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await userEvent.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
    await userEvent.click(boton('Buscar'))
    await userEvent.click(await screen.findByRole('button', { name: /Monferno/ }))
    await userEvent.click(boton('Cancelar'))
    expect(screen.queryByLabelText('Nivel')).toBeNull()
  })

  test('añadir guarda el equipo con nivel, movimientos y habilidad; quitar lo borra', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    const u = userEvent.setup()
    await u.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
    await u.click(boton('Buscar'))
    await u.click(await screen.findByRole('button', { name: /Monferno/ }))
    await u.type(screen.getByLabelText('Nivel'), '25')
    await u.selectOptions(screen.getByLabelText('Habilidad'), 'Iron Fist')
    await u.click(boton(/^Mach Punch ES/))
    await u.click(boton('Añadir al equipo'))

    expect(screen.getByText('1 / 6')).toBeTruthy()
    expect(screen.getByText('Monferno · Nv. 25')).toBeTruthy()
    const [guardado] = cargarEquipo('platino')
    expect(guardado).toMatchObject({ especie: 'Monferno', nivel: '25', movimientos: ['Mach Punch'], habilidad: 'Iron Fist' })
    expect((screen.getByLabelText('Buscar Pokémon') as HTMLInputElement).value).toBe('')

    await u.click(boton('Quitar'))
    expect(screen.getByText('0 / 6')).toBeTruthy()
    expect(cargarEquipo('platino')).toEqual([])
  })

  test('con 6 Pokémon no deja añadir más', async () => {
    const lleno = Array.from({ length: 6 }, () => ({
      ...nuevoMiembro({ especie: 'Monferno', nombre: 'Monferno', tipos: [] }),
      movimientos: ['Ember'],
    }))
    guardarEquipo('platino', lleno)
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    expect(screen.getByText('6 / 6')).toBeTruthy()
    await userEvent.type(screen.getByLabelText('Buscar Pokémon'), 'monf')
    await userEvent.click(boton('Buscar'))
    const resultados = await screen.findByLabelText('Resultados de búsqueda')
    expect((within(resultados).getByRole('button') as HTMLButtonElement).disabled).toBe(true)
  })

  test('un miembro guardado inválido muestra su error y bloquea el envío', async () => {
    guardarEquipo('platino', [{ ...nuevoMiembro({ especie: 'Monferno', nombre: 'Monferno', tipos: [] }, '300'), movimientos: ['Ember'] }])
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    expect(screen.getByText('Escribe un número entero entre 1 y 100.')).toBeTruthy()
    expect((boton('Simular combate') as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('enviar', () => {
  test('sin equipo el botón está desactivado', async () => {
    apiNormal()
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    expect((boton('Simular combate') as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText('Añade al menos un Pokémon a tu equipo.')).toBeTruthy()
  })

  test('crea la simulación, guarda el contexto y navega', async () => {
    apiNormal({ 'POST /simulaciones': json({ id: 77, estado: 'pendiente', posicion: 1, workerDisponible: true }, 202) })
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await añadirMonferno()
    await userEvent.click(boton('Simular combate'))
    expect(await screen.findByText('destino 77')).toBeTruthy()

    const post = llamadas.find((l) => l.init.method === 'POST')!
    const cuerpo = JSON.parse(post.init.body as string)
    expect(cuerpo.gen).toBe('gen4')
    expect(cuerpo.rival.especie).toBe('Cranidos')
    expect(cuerpo.candidatos).toEqual([{ especie: 'Monferno', movimientos: ['Mach Punch'], habilidad: 'Blaze' }])
    expect(cuerpo).not.toHaveProperty('simulaciones')

    expect(cargarContexto(77)).toMatchObject({ juego: 'platino', lider: 'Roark', rival: 2, nombres: { Monferno: 'Monferno', Cranidos: 'Cranidos' } })
  })

  test('429 con id → navega a la simulación en curso', async () => {
    apiNormal({ 'POST /simulaciones': json({ error: 'Ya tienes una en curso.', id: 12 }, 429) })
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await añadirMonferno()
    await userEvent.click(boton('Simular combate'))
    expect(await screen.findByText('destino 12')).toBeTruthy()
  })

  test('otro error → mensaje y se queda', async () => {
    apiNormal({ 'POST /simulaciones': json({ error: 'Petición inválida.' }, 400) })
    abrir()
    await screen.findByRole('button', { name: /Roark/ })
    await añadirMonferno()
    await userEvent.click(boton('Simular combate'))
    expect((await alerta()).textContent).toBe('Petición inválida.')
    await waitFor(() => expect((boton('Simular combate') as HTMLButtonElement).disabled).toBe(false))
  })
})
