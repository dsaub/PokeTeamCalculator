import { expect, test } from 'bun:test'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { Layout } from './Layout.tsx'

test('envuelve el contenido con cabecera, navegación y pie', () => {
  render(
    <MemoryRouter>
      <Layout>
        <p>contenido</p>
      </Layout>
    </MemoryRouter>,
  )
  expect(screen.getByRole('main').textContent).toBe('contenido')
  expect(screen.getByRole('navigation', { name: 'Principal' })).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Contrataque' }).getAttribute('href')).toBe('/')
  expect(screen.getByRole('link', { name: 'Nueva simulación' }).getAttribute('href')).toBe('/')
  expect(screen.getByRole('contentinfo').textContent).toContain('Nintendo')
})
