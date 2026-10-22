import { describe, expect, test } from 'bun:test'
import { render } from '@testing-library/react'
import { nombreTipo, TypeBadge } from './TypeBadge.tsx'

describe('nombreTipo', () => {
  test('traduce sin importar mayúsculas', () => {
    expect(nombreTipo('fire')).toBe('Fuego')
    expect(nombreTipo('Fire')).toBe('Fuego')
    expect(nombreTipo('FAIRY')).toBe('Hada')
  })

  test('un tipo desconocido se devuelve tal cual', () => {
    expect(nombreTipo('Stellar')).toBe('Stellar')
  })
})

describe('TypeBadge', () => {
  test('muestra el nombre, el color y el icono', () => {
    const { container } = render(<TypeBadge type="water" />)
    const span = container.querySelector('span')!
    expect(span.textContent).toBe('Agua')
    expect(span.className).toBe('type-badge type-badge-md')
    expect(span.getAttribute('style')).toContain('--type-color: #6390F0')
    expect(span.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
  })

  test('tamaño pequeño y texto claro en colores oscuros', () => {
    const { container } = render(<TypeBadge type="Fighting" size="sm" />)
    expect(container.querySelector('span')!.className).toBe('type-badge type-badge-sm on-color-light')
  })

  test('tipo desconocido: gris, sin icono y con el texto recibido', () => {
    const { container } = render(<TypeBadge type="???" />)
    const span = container.querySelector('span')!
    expect(span.textContent).toBe('???')
    expect(span.querySelector('svg')).toBeNull()
    expect(span.getAttribute('style')).toContain('var(--surface-muted)')
  })
})
