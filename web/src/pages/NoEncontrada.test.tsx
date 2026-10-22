import { expect, test } from 'bun:test'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { NoEncontrada } from './NoEncontrada.tsx'

test('explica el 404 y enlaza al inicio', () => {
  render(
    <MemoryRouter>
      <NoEncontrada />
    </MemoryRouter>,
  )
  expect(screen.getByRole('heading', { name: 'Esta página no existe' })).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Volver al inicio' }).getAttribute('href')).toBe('/')
})
