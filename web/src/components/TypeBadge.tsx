import type { CSSProperties, ReactNode } from 'react'

interface TipoVisual {
  label: string
  color: string
  light?: boolean
  icon?: ReactNode
}

// La API devuelve los tipos en minúsculas (`rock`, `fire`); también aceptamos
// la forma capitalizada por si llega de datos antiguos o del mock.
const TYPES: Record<string, TipoVisual> = {
  normal: { label: 'Normal', color: '#A8A77A', icon: <circle cx="12" cy="12" r="7" /> },
  fire: {
    label: 'Fuego',
    color: '#EE8130',
    icon: <path d="M12 3c1 4 6 6 6 11a6 6 0 0 1-12 0c0-3 2-5 3-7 1 2 1 3 2 4 1-3 1-5 1-8z" />,
  },
  water: { label: 'Agua', color: '#6390F0', icon: <path d="M12 3s6 7 6 11a6 6 0 0 1-12 0c0-4 6-11 6-11z" /> },
  electric: { label: 'Eléctrico', color: '#F7D02C', icon: <path d="M13 2 4 14h7l-1 8 9-12h-7z" /> },
  grass: { label: 'Planta', color: '#7AC74C', icon: <path d="M5 19C5 11 11 5 19 5c0 8-6 14-14 14zM5 19l8-8" /> },
  ice: { label: 'Hielo', color: '#96D9D6', icon: <path d="M12 3v18M4.2 7.5l15.6 9M4.2 16.5l15.6-9" /> },
  fighting: {
    label: 'Lucha',
    color: '#C22E28',
    light: true,
    icon: <path d="M6 10h12v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4zM9 10V6M12 10V5M15 10V6" />,
  },
  poison: {
    label: 'Veneno',
    color: '#A33EA1',
    light: true,
    icon: (
      <>
        <circle cx="9" cy="15" r="4" />
        <circle cx="16" cy="8" r="3" />
        <circle cx="17.5" cy="17" r="1.5" />
      </>
    ),
  },
  ground: { label: 'Tierra', color: '#E2BF65', icon: <path d="M3 19h18M5 19l5-8 3 4 2-3 4 7" /> },
  flying: { label: 'Volador', color: '#A98FF3', icon: <path d="M3 15c6 0 10-4 18-10-1 8-6 13-14 13zM3 15l3 4" /> },
  psychic: {
    label: 'Psíquico',
    color: '#F95587',
    icon: (
      <>
        <path d="M2 12s4-6 10-6 10 6 10 6-4 6-10 6S2 12 2 12z" />
        <circle cx="12" cy="12" r="3" />
      </>
    ),
  },
  bug: {
    label: 'Bicho',
    color: '#A6B91A',
    icon: (
      <>
        <ellipse cx="12" cy="14" rx="5" ry="6" />
        <path d="M12 8v12M9 4l1.5 4M15 4l-1.5 4" />
      </>
    ),
  },
  rock: { label: 'Roca', color: '#B6A136', icon: <path d="M4 17 7 8l6-3 6 5 1 7z" /> },
  ghost: {
    label: 'Fantasma',
    color: '#735797',
    light: true,
    icon: <path d="M6 20v-9a6 6 0 0 1 12 0v9l-2-2-2 2-2-2-2 2-2-2zM10 10v1M14 10v1" />,
  },
  dragon: { label: 'Dragón', color: '#6F35FC', light: true, icon: <path d="M4 18l5-5 3 3 8-11M14 5h6v6" /> },
  dark: { label: 'Siniestro', color: '#705746', light: true, icon: <path d="M20 14A8 8 0 1 1 10 4a6 6 0 0 0 10 10z" /> },
  steel: {
    label: 'Acero',
    color: '#B7B7CE',
    icon: (
      <>
        <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
        <circle cx="12" cy="12" r="3" />
      </>
    ),
  },
  fairy: { label: 'Hada', color: '#D685AD', icon: <path d="M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z" /> },
}

// Tipo desconocido: gris neutro sin icono.
const FALLBACK: TipoVisual = { label: '', color: 'var(--surface-muted)' }

function buscar(tipo: string): TipoVisual | undefined {
  return TYPES[tipo.toLowerCase()]
}

export function nombreTipo(tipo: string): string {
  return buscar(tipo)?.label ?? tipo
}

export function TypeBadge({ type, size = 'md' }: { type: string; size?: 'sm' | 'md' }) {
  const { label, color, light, icon } = buscar(type) ?? FALLBACK

  return (
    <span
      className={`type-badge type-badge-${size}${light ? ' on-color-light' : ''}`}
      style={{ '--type-color': color } as CSSProperties}
    >
      {icon && (
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          {icon}
        </svg>
      )}
      {label || type}
    </span>
  )
}
