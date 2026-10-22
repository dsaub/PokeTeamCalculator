import { describe, expect, test } from 'bun:test'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { json, rutas } from '../test/fetch.ts'
import { vaciarCacheCatalogo } from './api/catalogo.ts'
import { App } from './App.tsx'

function abrir(ruta: string) {
  render(
    <MemoryRouter initialEntries={[ruta]}>
      <App />
    </MemoryRouter>,
  )
}

describe('rutas', () => {
  test('/ → configurar', async () => {
    vaciarCacheCatalogo()
    rutas({ 'GET /juegos': [] })
    abrir('/')
    expect(screen.getByRole('heading', { name: '¿Quién puede ganar?' })).toBeTruthy()
    expect(await screen.findByText('No hay juegos con gimnasios disponibles.')).toBeTruthy()
  })

  test('/simulacion/:id → simulación', async () => {
    rutas({ 'GET /simulaciones/4': json({ id: 4, estado: 'cancelado' }) })
    abrir('/simulacion/4')
    expect(await screen.findByRole('heading', { name: 'Simulación cancelada' })).toBeTruthy()
  })

  test('cualquier otra → 404, dentro del layout', () => {
    abrir('/no/existe')
    expect(screen.getByRole('heading', { name: 'Esta página no existe' })).toBeTruthy()
    expect(screen.getByRole('navigation', { name: 'Principal' })).toBeTruthy()
  })
})
