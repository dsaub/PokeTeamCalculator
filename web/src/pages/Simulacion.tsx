import { useEffect, useReducer, useState } from 'react'
import { Link, useParams } from 'react-router'
import { ApiError } from '../api/cliente.ts'
import { abrirEventos, cancelarSimulacion, consultarSimulacion } from '../api/simulaciones.ts'
import { cargarContexto, type ContextoSimulacion } from '../dominio/almacen.ts'
import { ESTADO_INICIAL, enCurso, faseDeEstado, reducir } from '../dominio/maquina.ts'
import { seguirSimulacion } from '../dominio/seguimiento.ts'
import {
  decimal,
  explicacionEmpate,
  fraseRecomendacion,
  mensajeDeError,
  porcentaje,
  textoPosicion,
  tiempoEstimado,
} from '../dominio/textos.ts'

const ETIQUETA_CONEXION: Record<string, string> = {
  conectando: 'Conectando…',
  conectado: 'En directo',
  reconectando: 'Reconectando…',
  sondeo: 'Actualizando por sondeo',
  inactiva: '',
}

export function Simulacion() {
  const { id } = useParams()
  const numero = Number(id)
  const [estado, despachar] = useReducer(reducir, ESTADO_INICIAL)
  const [contexto, setContexto] = useState<ContextoSimulacion | null>(null)
  const [intento, setIntento] = useState(0)

  useEffect(() => {
    if (Number.isInteger(numero) && numero > 0) setContexto(cargarContexto(numero))
  }, [numero])

  useEffect(() => {
    if (!Number.isInteger(numero) || numero <= 0) {
      despachar({ tipo: 'consultaFallida', status: 404, mensaje: 'Simulación no encontrada' })
      return
    }

    let activo = true
    let parar = () => {}

    void (async () => {
      try {
        const actual = await consultarSimulacion(numero)
        if (!activo) return
        despachar({ tipo: 'estado', estado: actual })
        if (enCurso(faseDeEstado(actual))) {
          parar = seguirSimulacion(numero, despachar, { abrirEventos, consultar: consultarSimulacion })
        }
      } catch (e) {
        if (!activo) return
        despachar({
          tipo: 'consultaFallida',
          status: e instanceof ApiError ? e.status : 0,
          mensaje: mensajeDeError(e),
        })
      }
    })()

    return () => {
      activo = false
      parar()
    }
  }, [numero, intento])

  async function cancelar() {
    if (!window.confirm('¿Seguro que quieres cancelar la simulación?')) return
    despachar({ tipo: 'cancelando' })
    try {
      await cancelarSimulacion(numero)
      despachar({ tipo: 'cancelada' })
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        try {
          despachar({ tipo: 'estado', estado: await consultarSimulacion(numero) })
          return
        } catch {
          // Si tampoco se puede consultar, mostramos el fallo de cancelación.
        }
      }
      despachar({ tipo: 'cancelacionFallida', mensaje: mensajeDeError(e) })
    }
  }

  const fase = estado.fase
  const nombre = (especie: string) => contexto?.nombres[especie] ?? especie

  if (fase.tipo === 'noEncontrada') {
    return (
      <section className="not-found stack">
        <p className="eyebrow">Simulación</p>
        <h1 className="title">No encontrada</h1>
        <p className="lead">Esta simulación no existe o pertenece a otra sesión del navegador.</p>
        <p>
          <Link className="btn btn-dark" to="/">
            Preparar otro combate
          </Link>
        </p>
      </section>
    )
  }

  if (fase.tipo === 'falloCarga') {
    return (
      <section className="status-card card stack" style={{ padding: 24 }}>
        <p className="eyebrow">Sin conexión</p>
        <h1 className="h-large">No se pudo cargar la simulación</h1>
        <p className="lead">{fase.mensaje}</p>
        <p>
          <button
            type="button"
            className="btn btn-accent"
            onClick={() => {
              despachar({ tipo: 'reintentar' })
              setIntento((n) => n + 1)
            }}
          >
            Reintentar
          </button>
        </p>
      </section>
    )
  }

  if (fase.tipo === 'cargando') {
    return (
      <section className="status-card card stack" style={{ padding: 24 }}>
        <p className="eyebrow">Simulación</p>
        <h1 className="h-large">Cargando…</h1>
        <p className="lead">Consultando el estado de la simulación.</p>
      </section>
    )
  }

  if (fase.tipo === 'pendiente' || fase.tipo === 'procesando') {
    return (
      <section className="card stack" style={{ padding: 28, gap: 18 }}>
        <div className="row-between wrap">
          <div className="stack">
            <p className="eyebrow">Simulación #{numero}</p>
            <h1 className="h-large">
              {fase.tipo === 'pendiente' ? 'En cola' : 'Simulando combates…'}
            </h1>
          </div>
          <span className="status-chip starting">{ETIQUETA_CONEXION[estado.conexion] || 'En curso'}</span>
        </div>

        {fase.tipo === 'pendiente' ? (
          <div className="stack gap-8">
            <p className="lead">{textoPosicion(fase.posicion)}.</p>
            <p className="muted">
              Tiempo estimado: {tiempoEstimado(fase.estimadoSegundos)}. Posición {fase.posicion}.
            </p>
            <span className="bar thick" aria-hidden="true">
              <span
                className="bar-fill accent"
                style={{ width: `${Math.max(8, Math.min(100, 100 / Math.max(1, fase.posicion)))}%` }}
              />
            </span>
          </div>
        ) : (
          <p className="lead">
            El worker está calculando los combates de tu equipo. Puedes dejar esta pestaña abierta:
            el resultado aparecerá aquí.
          </p>
        )}

        {!estado.workerDisponible && (
          <p className="warn small">
            El worker no está disponible ahora mismo. La simulación sigue en cola y se procesará
            cuando vuelva.
          </p>
        )}

        {estado.cancelacion.tipo === 'fallo' && <p className="warn small">{estado.cancelacion.mensaje}</p>}

        <div className="row wrap gap-8">
          <Link className="btn btn-light" to="/">
            Cambiar equipo
          </Link>
          <button
            type="button"
            className="btn btn-light"
            onClick={() => void cancelar()}
            disabled={estado.cancelacion.tipo === 'enviando'}
          >
            {estado.cancelacion.tipo === 'enviando' ? 'Cancelando…' : 'Cancelar simulación'}
          </button>
        </div>
      </section>
    )
  }

  if (fase.tipo === 'error') {
    return (
      <section className="status-card card stack" style={{ padding: 24 }}>
        <p className="eyebrow">Simulación #{numero}</p>
        <h1 className="h-large">No se pudo completar</h1>
        <p className="lead">{fase.mensaje}</p>
        <p className="muted">
          Revisa que los Pokémon y movimientos existan en esta generación y vuelve a intentarlo.
        </p>
        <p>
          <Link className="btn btn-dark" to="/">
            Preparar otro combate
          </Link>
        </p>
      </section>
    )
  }

  if (fase.tipo === 'cancelado') {
    return (
      <section className="status-card card stack" style={{ padding: 24 }}>
        <p className="eyebrow">Simulación #{numero}</p>
        <h1 className="h-large">Simulación cancelada</h1>
        <p className="lead">
          {fase.porUsuario
            ? 'La has cancelado tú. No se mostrará ningún resultado.'
            : 'La simulación se canceló antes de terminar.'}
        </p>
        <p>
          <Link className="btn btn-dark" to="/">
            Preparar otro combate
          </Link>
        </p>
      </section>
    )
  }

  const resultado = fase.resultado
  const mejor = resultado.resultados[0]
  const segundo = resultado.resultados[1]

  return (
    <section className="stack" style={{ gap: 28 }}>
      <div className="page-head">
        <div className="grow stack">
          <p className="eyebrow">Simulación #{numero}</p>
          <h1 className="title">Resultado</h1>
          <p className="lead">
            {resultado.simulaciones} combates contra {resultado.rival}
            {contexto ? ` (${contexto.lider})` : ''}. {fase.cache ? 'Resultado recuperado de la caché.' : ''}
          </p>
        </div>
      </div>

      {!mejor ? (
        <p className="card" style={{ padding: 20 }}>
          La simulación terminó sin resultados.
        </p>
      ) : (
        <>
          <section className="card tool-section best">
            <div className="best-head">
              <div className="best-name">
                <p>{nombre(mejor.pokemon)}</p>
                <p className="mono-meta on-dark">
                  {mejor.pokemon} · Nv. {mejor.nivel}
                </p>
              </div>
              <div className="best-score">
                <span>{porcentaje(mejor.winrate)}</span>
                <span className="small">victorias</span>
              </div>
            </div>
            <p>{fraseRecomendacion(mejor)}</p>
            <ul className="best-facts">
              <li>
                <span className="small">Victorias</span>
                <span className="strong">
                  {mejor.victorias} / {resultado.simulaciones}
                </span>
              </li>
              <li>
                <span className="small">PS medios al ganar</span>
                <span className="strong">{porcentaje(mejor.psMedioVictoria)}</span>
              </li>
              <li>
                <span className="small">Turnos medios</span>
                <span className="strong">{decimal(mejor.turnosMedio)}</span>
              </li>
              <li>
                <span className="small">Empates</span>
                <span className="strong">{mejor.empates}</span>
              </li>
            </ul>
          </section>

          {segundo && (
            <p className="card" style={{ padding: 18 }}>
              {explicacionEmpate(mejor, segundo, nombre) ?? (
                <>
                  El segundo mejor es <strong>{nombre(segundo.pokemon)}</strong>, con{' '}
                  {porcentaje(segundo.winrate)} de victorias.
                </>
              )}
            </p>
          )}

          <section className="card stack" style={{ padding: 24, gap: 16 }}>
            <div className="row-between wrap">
              <h2 className="h-medium">Ranking completo</h2>
              <span className="mono-meta">{resultado.ms} ms de cálculo</span>
            </div>
            <ol className="ranking">
              {resultado.resultados.map((r, i) => (
                <li key={`${r.pokemon}-${i}`}>
                  <span className="mono-meta">{i + 1}</span>
                  <span className="strong">{nombre(r.pokemon)}</span>
                  <span className="bar" aria-hidden="true">
                    <span className="bar-fill accent" style={{ width: `${r.winrate * 100}%` }} />
                  </span>
                  <span className="mono-meta score">{porcentaje(r.winrate)}</span>
                </li>
              ))}
            </ol>
          </section>

          {contexto && (
            <section className="card stack" style={{ padding: 24, gap: 12 }}>
              <h2 className="h-medium">Equipo y rival simulados</h2>
              <div className="chips">
                {contexto.peticion.candidatos.map((c, i) => (
                  <span key={`${c.especie}-${i}`} className="chip chip-done">
                    {nombre(c.especie)}
                  </span>
                ))}
                <span className="chip">{contexto.peticion.rival.especie}</span>
              </div>
              <p className="small muted">
                Movimientos del rival: {contexto.peticion.rival.movimientos.join(' · ')}
              </p>
            </section>
          )}
        </>
      )}

      <div className="row wrap gap-8">
        <Link className="btn btn-dark" to="/">
          Preparar otro combate
        </Link>
        {contexto && (
          <button type="button" className="btn btn-light" onClick={() => window.location.assign('/')}>
            Cambiar equipo
          </button>
        )}
      </div>
    </section>
  )
}
