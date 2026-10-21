import type { ReactNode } from 'react'
import { Link } from 'react-router'

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="page">
      <header className="site-header">
        <nav className="container nav-row" aria-label="Principal">
          <Link className="logo" to="/">
            <svg viewBox="0 0 32 32" width="28" height="28" aria-hidden="true">
              <circle cx="16" cy="16" r="13" fill="none" stroke="currentColor" strokeWidth="3" />
              <path d="M3 16h26" stroke="currentColor" strokeWidth="3" />
              <circle cx="16" cy="16" r="5" fill="var(--paper)" stroke="currentColor" strokeWidth="3" />
            </svg>
            Contrataque
          </Link>
          <div className="nav-links">
            <Link to="/">Nueva simulación</Link>
          </div>
        </nav>
      </header>
      <main className="container main">{children}</main>
      <footer className="site-footer">
        <div className="container">
          <span>Simulador de combates contra líderes de gimnasio.</span>
          <span className="mono-meta">Datos de Pokémon © Nintendo / Game Freak</span>
        </div>
      </footer>
    </div>
  )
}
