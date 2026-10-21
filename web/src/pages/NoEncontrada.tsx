import { Link } from 'react-router'

export function NoEncontrada() {
  return (
    <section className="not-found stack">
      <p className="eyebrow">Error 404</p>
      <h1 className="title">Esta página no existe</h1>
      <p className="lead">Puede que el enlace esté mal escrito o que la simulación ya no esté disponible.</p>
      <p>
        <Link className="btn btn-dark" to="/">
          Volver al inicio
        </Link>
      </p>
    </section>
  )
}
