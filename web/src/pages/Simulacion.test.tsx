import { afterEach, beforeEach, describe, expect, spyOn, test } from 'bun:test'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { falsearEventSource, FuenteFalsa, json, rutas } from '../../test/fetch.ts'
import type { EstadoSimulacion, Resultado, ResultadoPokemon } from '../api/tipos.ts'
import { guardarContexto, type ContextoSimulacion } from '../dominio/almacen.ts'
import { Simulacion } from './Simulacion.tsx'

const fila = (c: Partial<ResultadoPokemon>): ResultadoPokemon => ({
  pokemon: 'Monferno',
  nivel: 20,
  victorias: 270,
  derrotas: 30,
  empates: 0,
  winrate: 0.9,
  psMedioVictoria: 0.6,
  turnosMedio: 3,
  ...c,
})

const resultado = (filas: ResultadoPokemon[]): Resultado => ({
  gen: 'gen4',
  rival: 'Cranidos (Nv. 14)',
  simulaciones: 300,
  resultados: filas,
  ms: 1234,
})

let confirmar: ReturnType<typeof spyOn<Window, 'confirm'>>

beforeEach(() => {
  falsearEventSource()
  confirmar = spyOn(window, 'confirm').mockReturnValue(true)
})

afterEach(() => confirmar.mockRestore())

function abrir(id: string) {
  return render(
    <MemoryRouter initialEntries={[`/simulacion/${id}`]}>
      <Routes>
        <Route path="/simulacion/:id" element={<Simulacion />} />
      </Routes>
    </MemoryRouter>,
  )
}

function conEstado(estado: EstadoSimulacion | Response) {
  return rutas({ [`GET /simulaciones/${'id' in estado ? estado.id : 1}`]: estado instanceof Response ? estado : json(estado) })
}

describe('carga', () => {
  test('id no numérico → no encontrada sin llamar a la API', async () => {
    const llamadas = rutas({})
    abrir('abc')
    expect(await screen.findByRole('heading', { name: 'No encontrada' })).toBeTruthy()
    expect(llamadas).toHaveLength(0)
  })

  test('id 0 → no encontrada', async () => {
    rutas({})
    abrir('0')
    expect(await screen.findByRole('heading', { name: 'No encontrada' })).toBeTruthy()
  })

  test('muestra «Cargando…» mientras consulta', () => {
    rutas({ 'GET /simulaciones/1': () => new Promise<Response>(() => {}) })
    abrir('1')
    expect(screen.getByRole('heading', { name: 'Cargando…' })).toBeTruthy()
  })

  test('404 de la API → no encontrada', async () => {
    rutas({ 'GET /simulaciones/1': json({ error: 'No existe' }, 404) })
    abrir('1')
    expect(await screen.findByRole('heading', { name: 'No encontrada' })).toBeTruthy()
  })

  test('fallo de red → reintentar vuelve a consultar', async () => {
    let n = 0
    rutas({
      'GET /simulaciones/1': () => {
        n++
        if (n === 1) throw new TypeError('offline')
        return json({ id: 1, estado: 'error', error: 'Movimiento inválido' })
      },
    })
    abrir('1')
    expect(await screen.findByRole('heading', { name: 'No se pudo cargar la simulación' })).toBeTruthy()
    expect(screen.getByText(/No se pudo conectar/)).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    expect(await screen.findByRole('heading', { name: 'No se pudo completar' })).toBeTruthy()
    expect(screen.getByText('Movimiento inválido')).toBeTruthy()
    expect(n).toBe(2)
  })
})

describe('en curso', () => {
  test('pendiente: posición, estimación y seguimiento por SSE', async () => {
    conEstado({ id: 3, estado: 'pendiente', posicion: 3, estimadoSegundos: 30 })
    abrir('3')
    expect(await screen.findByRole('heading', { name: 'En cola' })).toBeTruthy()
    expect(screen.getByText('Hay 2 por delante en la cola.')).toBeTruthy()
    expect(screen.getByText(/unos 30 segundos\. Posición 3\./)).toBeTruthy()
    expect(FuenteFalsa.abiertas.map((f) => f.url)).toEqual(['/api/simulaciones/3/eventos'])
    expect(screen.getByText('Conectando…')).toBeTruthy()

    const f = FuenteFalsa.abiertas[0]!
    act(() => f.abrir())
    expect(screen.getByText('En directo')).toBeTruthy()
    act(() => f.emitir('procesando', {}))
    expect(screen.getByRole('heading', { name: 'Simulando combates…' })).toBeTruthy()
    act(() => f.emitir('completado', { resultado: resultado([fila({})]) }))
    expect(screen.getByRole('heading', { name: 'Resultado' })).toBeTruthy()
    expect(f.cerrada).toBe(true)
  })

  test('al desmontar cierra el SSE', async () => {
    conEstado({ id: 3, estado: 'procesando' })
    const { unmount } = abrir('3')
    await screen.findByRole('heading', { name: 'Simulando combates…' })
    unmount()
    expect(FuenteFalsa.abiertas[0]!.cerrada).toBe(true)
  })

  test('un estado final no abre SSE', async () => {
    conEstado({ id: 3, estado: 'cancelado' })
    abrir('3')
    expect(await screen.findByText('La simulación se canceló antes de terminar.')).toBeTruthy()
    expect(FuenteFalsa.abiertas).toHaveLength(0)
  })
})

describe('cancelar', () => {
  test('si no se confirma no hace nada', async () => {
    confirmar.mockReturnValue(false)
    const llamadas = conEstado({ id: 5, estado: 'procesando' })
    abrir('5')
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar simulación' }))
    expect(llamadas.filter((l) => l.init.method === 'DELETE')).toHaveLength(0)
  })

  test('204 → cancelada por el jugador', async () => {
    const llamadas = rutas({
      'GET /simulaciones/5': json({ id: 5, estado: 'procesando' }),
      'DELETE /simulaciones/5': new Response(null, { status: 204 }),
    })
    abrir('5')
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar simulación' }))
    expect(await screen.findByText('La has cancelado tú. No se mostrará ningún resultado.')).toBeTruthy()
    expect(confirmar).toHaveBeenCalledTimes(1)
    expect(llamadas.some((l) => l.init.method === 'DELETE')).toBe(true)
  })

  test('409 → consulta el estado real', async () => {
    let consultas = 0
    rutas({
      'GET /simulaciones/5': () =>
        json(++consultas === 1 ? { id: 5, estado: 'procesando' } : { id: 5, estado: 'completado', resultado: resultado([]) }),
      'DELETE /simulaciones/5': json({ error: 'Ya ha terminado.' }, 409),
    })
    abrir('5')
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar simulación' }))
    expect(await screen.findByText('La simulación terminó sin resultados.')).toBeTruthy()
  })

  test('409 y la consulta también falla → muestra el fallo y sigue', async () => {
    let consultas = 0
    rutas({
      'GET /simulaciones/5': () => (++consultas === 1 ? json({ id: 5, estado: 'procesando' }) : json({ error: 'caído' }, 500)),
      'DELETE /simulaciones/5': json({ error: 'Ya ha terminado.' }, 409),
    })
    abrir('5')
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar simulación' }))
    expect(await screen.findByText('Ya ha terminado.')).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Simulando combates…' })).toBeTruthy()
  })

  test('otro error → mensaje y se puede volver a intentar', async () => {
    rutas({
      'GET /simulaciones/5': json({ id: 5, estado: 'pendiente', posicion: 1, estimadoSegundos: 3 }),
      'DELETE /simulaciones/5': json({ error: 'No se pudo cancelar.' }, 500),
    })
    abrir('5')
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar simulación' }))
    expect(await screen.findByText('No se pudo cancelar.')).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Cancelar simulación' }) as HTMLButtonElement).disabled).toBe(false)
  })
})

describe('resultado', () => {
  const contexto: ContextoSimulacion = {
    juego: 'platino',
    lider: 'Roark',
    rival: 2,
    peticion: {
      gen: 'gen4',
      rival: { especie: 'Cranidos', nivel: 14, movimientos: ['Headbutt', 'Pursuit'] },
      candidatos: [
        { especie: 'Monferno', movimientos: ['Mach Punch'] },
        { especie: 'Grotle', movimientos: ['Razor Leaf'] },
      ],
    },
    nombres: { Monferno: 'Monferno ES', Grotle: 'Grotle ES', Cranidos: 'Cranidos' },
  }

  test('ranking, recomendación y contexto guardado', async () => {
    guardarContexto(8, contexto)
    conEstado({
      id: 8,
      estado: 'completado',
      cache: true,
      resultado: resultado([fila({}), fila({ pokemon: 'Grotle', winrate: 0.5, victorias: 150 })]),
    })
    abrir('8')
    expect(await screen.findByRole('heading', { name: 'Resultado' })).toBeTruthy()
    expect(screen.getByText(/300 combates contra Cranidos \(Nv\. 14\) \(Roark\)\. Resultado recuperado de la caché\./)).toBeTruthy()
    expect(screen.getAllByText('Monferno ES').length).toBeGreaterThan(0)
    expect(screen.getByText('270 / 300')).toBeTruthy()
    expect(screen.getByText('1234 ms de cálculo')).toBeTruthy()
    expect(screen.getByText(/El segundo mejor es/).textContent).toContain('Grotle ES')
    expect(screen.getByText('Movimientos del rival: Headbutt · Pursuit')).toBeTruthy()
    expect(screen.getAllByRole('listitem').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByRole('button', { name: 'Cambiar equipo' })).toBeTruthy()
  })

  test('empate: explica por qué va primero', async () => {
    conEstado({
      id: 9,
      estado: 'completado',
      resultado: resultado([fila({ psMedioVictoria: 0.8 }), fila({ pokemon: 'Grotle', psMedioVictoria: 0.3 })]),
    })
    abrir('9')
    expect(await screen.findByText(/le quedan más PS al ganar/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Cambiar equipo' })).toBeNull()
  })

  test('error de la simulación', async () => {
    conEstado({ id: 2, estado: 'error', error: 'Candidato 1: especie desconocida' })
    abrir('2')
    expect(await screen.findByText('Candidato 1: especie desconocida')).toBeTruthy()
  })

  test('aviso cuando no hay worker', async () => {
    conEstado({ id: 3, estado: 'pendiente', posicion: 1, estimadoSegundos: 0 })
    abrir('3')
    await screen.findByRole('heading', { name: 'En cola' })
    expect(screen.queryByText(/El worker no está disponible/)).toBeNull()
    expect(screen.getByText('Eres el siguiente en la cola.')).toBeTruthy()
  })
})
