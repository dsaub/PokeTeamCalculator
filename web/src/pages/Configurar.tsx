import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { ApiError } from '../api/cliente.ts'
import { buscarPokemon, obtenerJuegos, obtenerLideres, obtenerPokemon } from '../api/catalogo.ts'
import type { Juego, Lider, PokemonDetalle, PokemonResumen } from '../api/tipos.ts'
import { crearSimulacion } from '../api/simulaciones.ts'
import { TypeBadge } from '../components/TypeBadge.tsx'
import { cargarEquipo, cargarUltimoJuego, guardarContexto, guardarEquipo, guardarUltimoJuego } from '../dominio/almacen.ts'
import { MAX_EQUIPO, nuevoMiembro, type MiembroEquipo } from '../dominio/equipo.ts'
import { construirPeticion } from '../dominio/peticion.ts'
import { validarEquipo } from '../dominio/validacion.ts'

function mensajeDeError(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return 'Ha ocurrido un error inesperado.'
}

export function Configurar() {
  const navegar = useNavigate()
  const [juegos, setJuegos] = useState<Juego[]>([])
  const [juego, setJuego] = useState<Juego | null>(null)
  const [lideres, setLideres] = useState<Lider[] | null>(null)
  const [lider, setLider] = useState<Lider | null>(null)
  const [rivalIdx, setRivalIdx] = useState(0)
  const [equipo, setEquipo] = useState<MiembroEquipo[]>([])
  const [busqueda, setBusqueda] = useState('')
  const [resultados, setResultados] = useState<PokemonResumen[]>([])
  const [buscando, setBuscando] = useState(false)
  const [seleccion, setSeleccion] = useState<PokemonDetalle | null>(null)
  const [cargandoDetalle, setCargandoDetalle] = useState(false)
  const [nivel, setNivel] = useState('')
  const [movimientos, setMovimientos] = useState<string[]>([])
  const [habilidad, setHabilidad] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [enviando, setEnviando] = useState(false)

  useEffect(() => {
    let activo = true
    obtenerJuegos()
      .then((lista) => {
        if (!activo) return
        setJuegos(lista)
        const guardado = cargarUltimoJuego()
        const inicial = lista.find((j) => j.id === guardado) ?? lista[0]
        if (!inicial) {
          setError('No hay juegos con gimnasios disponibles.')
          return
        }
        setJuego(inicial)
        setEquipo(cargarEquipo(inicial.id))
      })
      .catch((e) => {
        if (activo) setError(mensajeDeError(e))
      })
    return () => {
      activo = false
    }
  }, [])

  useEffect(() => {
    if (!juego) return
    let activo = true
    setLideres(null)
    setLider(null)
    setRivalIdx(0)
    setSeleccion(null)
    setResultados([])
    setBusqueda('')
    setError(null)

    obtenerLideres(juego.id)
      .then((lista) => {
        if (!activo) return
        setLideres(lista)
        const primero = lista[0]
        if (primero) {
          setLider(primero)
          setRivalIdx(Math.max(0, primero.equipo.length - 1))
        }
      })
      .catch((e) => {
        if (activo) setError(mensajeDeError(e))
      })
    return () => {
      activo = false
    }
  }, [juego])

  function elegirJuego(siguiente: Juego) {
    setJuego(siguiente)
    guardarUltimoJuego(siguiente.id)
    setEquipo(cargarEquipo(siguiente.id))
  }

  function elegirLider(siguiente: Lider) {
    setLider(siguiente)
    setRivalIdx(Math.max(0, siguiente.equipo.length - 1))
  }

  async function buscar(e: FormEvent) {
    e.preventDefault()
    if (!juego) return
    if (busqueda.trim().length < 2) {
      setError('Escribe al menos dos caracteres para buscar.')
      return
    }
    setBuscando(true)
    setError(null)
    try {
      setResultados(await buscarPokemon(juego.gen, busqueda))
    } catch (err) {
      setError(mensajeDeError(err))
    } finally {
      setBuscando(false)
    }
  }

  async function elegirPokemon(resumen: PokemonResumen) {
    if (!juego) return
    setCargandoDetalle(true)
    setError(null)
    try {
      const detalle = await obtenerPokemon(juego.gen, resumen.especie)
      setSeleccion(detalle)
      setNivel('')
      setMovimientos([])
      setHabilidad(detalle.habilidades[0]?.id ?? '')
    } catch (err) {
      setError(mensajeDeError(err))
    } finally {
      setCargandoDetalle(false)
    }
  }

  function alternarMovimiento(id: string) {
    setMovimientos((actual) => {
      if (actual.includes(id)) return actual.filter((m) => m !== id)
      if (actual.length >= 4) return actual
      return [...actual, id]
    })
  }

  function agregarAlEquipo() {
    if (!juego || !seleccion) return
    if (equipo.length >= MAX_EQUIPO) {
      setError(`El equipo admite como mucho ${MAX_EQUIPO} Pokémon.`)
      return
    }
    if (movimientos.length === 0) {
      setError('Elige al menos un movimiento.')
      return
    }
    const miembro = nuevoMiembro(
      { especie: seleccion.especie, nombre: seleccion.nombre, tipos: seleccion.tipos },
      nivel.trim(),
    )
    miembro.movimientos = movimientos
    miembro.habilidad = habilidad
    const nuevo = [...equipo, miembro]
    setEquipo(nuevo)
    guardarEquipo(juego.id, nuevo)
    setSeleccion(null)
    setResultados([])
    setBusqueda('')
    setError(null)
  }

  function quitarDelEquipo(uid: string) {
    if (!juego) return
    const nuevo = equipo.filter((m) => m.uid !== uid)
    setEquipo(nuevo)
    guardarEquipo(juego.id, nuevo)
  }

  const validacion = juego ? validarEquipo(juego.gen, equipo) : null

  async function enviar() {
    if (!juego || !lider) {
      setError('Elige un juego y un líder antes de simular.')
      return
    }
    if (!validacion?.valido) {
      setError(validacion?.general ?? 'Revisa el equipo antes de simular.')
      return
    }
    const rival = lider.equipo[rivalIdx]
    if (!rival) {
      setError('Elige el Pokémon del líder que será el rival.')
      return
    }

    setEnviando(true)
    setError(null)
    try {
      const peticion = construirPeticion(juego.gen, rival, equipo)
      const respuesta = await crearSimulacion(peticion)
      const nombres = Object.fromEntries(
        [rival, ...equipo].map((p) => [p.especie, p.nombre] as const),
      )
      guardarContexto(respuesta.id, {
        juego: juego.id,
        lider: lider.nombre,
        rival: rivalIdx,
        peticion,
        nombres,
      })
      navegar(`/simulacion/${respuesta.id}`)
    } catch (err) {
      if (err instanceof ApiError && err.idSimulacion) {
        navegar(`/simulacion/${err.idSimulacion}`)
        return
      }
      setError(mensajeDeError(err))
    } finally {
      setEnviando(false)
    }
  }

  return (
    <>
      <div className="page-head">
        <div className="grow stack">
          <p className="eyebrow">Prepara el combate</p>
          <h1 className="title">¿Quién puede ganar?</h1>
          <p className="lead">
            Elige el líder, monta tu equipo y simula cada Pokémon contra su rival. La API probará
            cientos de combates y te dirá cuál tiene más probabilidades.
          </p>
        </div>
      </div>

      {error && (
        <p className="card warn" role="alert" style={{ padding: 16 }}>
          {error}
        </p>
      )}

      <section className="equipo-layout">
        <div className="games">
          <h2 className="h-medium">Juego</h2>
          <ul>
            {juegos.map((j) => (
              <li key={j.id}>
                <button
                  type="button"
                  className="game-button"
                  aria-pressed={juego?.id === j.id}
                  onClick={() => elegirJuego(j)}
                >
                  <span className="stack">
                    <span className="strong">{j.nombre}</span>
                    <span className="small muted">{j.gen}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {juegos.length === 0 && !error && <p className="muted small">Cargando juegos…</p>}
        </div>

        <div className="card team-panel">
          <div className="row-between wrap">
            <h2 className="h-medium">Líder y rival</h2>
            {juego && <span className="mono-meta">{juego.nombre}</span>}
          </div>

          {lideres === null && juego && <p className="muted">Cargando líderes…</p>}
          {lideres?.length === 0 && <p className="muted">Este juego no tiene líderes de gimnasio.</p>}

          <ul className="leader-grid">
            {lideres?.map((l) => (
              <li key={l.id}>
                <button
                  type="button"
                  className={`leader-card${lider?.id === l.id ? ' is-next' : ''}`}
                  aria-pressed={lider?.id === l.id}
                  onClick={() => elegirLider(l)}
                >
                  <span className="row-between wrap">
                    <span className="leader-name">{l.nombre}</span>
                    <span className="mono-meta">{l.ciudad}</span>
                  </span>
                  <span className="row wrap gap-8">
                    {l.tipo && <TypeBadge type={l.tipo} size="sm" />}
                    <span className="mono-meta">
                      Nv. {l.equipo.length ? Math.max(...l.equipo.map((p) => p.nivel)) : '?'}
                    </span>
                  </span>
                  <span className="small muted">
                    {l.equipo.map((p) => p.nombre).join(' · ')}
                  </span>
                </button>
              </li>
            ))}
          </ul>

          {lider && (
            <div className="stack">
              <h3 className="h-small">Rival</h3>
              <div className="chips">
                {lider.equipo.map((p, i) => (
                  <button
                    key={`${p.especie}-${i}`}
                    type="button"
                    className="chip"
                    aria-pressed={rivalIdx === i}
                    onClick={() => setRivalIdx(i)}
                  >
                    {p.nombre} · Nv. {p.nivel}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </section>

      {juego && (
        <section className="card team-panel">
          <div className="row-between wrap">
            <h2 className="h-medium">Tu equipo</h2>
            <span className="mono-meta">{equipo.length} / 6</span>
          </div>

          <form className="add-form" onSubmit={buscar}>
            <div className="field grow">
              <label htmlFor="busqueda">Buscar Pokémon</label>
              <input
                id="busqueda"
                value={busqueda}
                onChange={(e) => setBusqueda(e.target.value)}
                placeholder="Al menos dos caracteres: monf, pika…"
                autoComplete="off"
              />
            </div>
            <button className="btn btn-light" type="submit" disabled={buscando}>
              {buscando ? 'Buscando…' : 'Buscar'}
            </button>
          </form>

          {resultados.length > 0 && (
            <div className="chips" aria-label="Resultados de búsqueda">
              {resultados.map((r) => (
                <button
                  key={r.especie}
                  type="button"
                  className="select-card"
                  onClick={() => void elegirPokemon(r)}
                  disabled={equipo.length >= 6 || cargandoDetalle}
                >
                  <span className="stack">
                    <span className="strong">{r.nombre}</span>
                    <span className="mono-meta">{r.especie}</span>
                  </span>
                  <span className="row wrap gap-6">
                    {r.tipos.map((t) => (
                      <TypeBadge key={t} type={t} size="sm" />
                    ))}
                  </span>
                </button>
              ))}
            </div>
          )}

          {seleccion && (
            <div className="card" style={{ padding: 18 }}>
              <div className="stack gap-12">
                <div className="row-between wrap">
                  <h3 className="h-small">{seleccion.nombre}</h3>
                  <span className="mono-meta">{seleccion.especie}</span>
                </div>
                <div className="row wrap gap-6">
                  {seleccion.tipos.map((t) => (
                    <TypeBadge key={t} type={t} size="sm" />
                  ))}
                </div>

                <div className="add-form">
                  <div className="field level-field">
                    <label htmlFor="nivel">Nivel</label>
                    <input
                      id="nivel"
                      inputMode="numeric"
                      value={nivel}
                      onChange={(e) => setNivel(e.target.value)}
                      placeholder="100"
                    />
                  </div>
                  {seleccion.habilidades.length > 0 && (
                    <div className="field grow">
                      <label htmlFor="habilidad">Habilidad</label>
                      <select id="habilidad" value={habilidad} onChange={(e) => setHabilidad(e.target.value)}>
                        <option value="">La primera</option>
                        {seleccion.habilidades.map((h) => (
                          <option key={h.id} value={h.id}>
                            {h.nombre}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                </div>

                <div className="stack gap-8">
                  <span className="small strong">Movimientos ({movimientos.length} / 4)</span>
                  <div className="chips">
                    {seleccion.movimientos.map((m) => (
                      <button
                        key={m.id}
                        type="button"
                        className="chip"
                        aria-pressed={movimientos.includes(m.id)}
                        onClick={() => alternarMovimiento(m.id)}
                      >
                        {m.nombre}
                        <span className="mono-meta"> · {m.tipo}</span>
                      </button>
                    ))}
                  </div>
                </div>

                <div className="row wrap gap-8">
                  <button type="button" className="btn btn-accent" onClick={agregarAlEquipo}>
                    Añadir al equipo
                  </button>
                  <button type="button" className="btn btn-light" onClick={() => setSeleccion(null)}>
                    Cancelar
                  </button>
                </div>
              </div>
            </div>
          )}

          <ul className="slots">
            {equipo.map((m, i) => (
              <li key={m.uid} className="slot">
                <div className="slot-head">
                  <span className="avatar" style={{ width: 40, height: 40 }} aria-hidden="true">
                    {m.nombre.slice(0, 1)}
                  </span>
                  <span className="stack grow">
                    <span className="strong">{m.nombre}</span>
                    <span className="mono-meta">
                      {m.especie} · Nv. {m.nivel || '100'}
                    </span>
                  </span>
                </div>
                <div className="row wrap gap-6">
                  {m.tipos.map((t) => (
                    <TypeBadge key={t} type={t} size="sm" />
                  ))}
                </div>
                <p className="small muted">{m.movimientos.join(' · ')}</p>
                <button type="button" className="chip remove" onClick={() => quitarDelEquipo(m.uid)}>
                  Quitar
                </button>
                {validacion?.porMiembro[i] && (
                  <p className="warn small">{Object.values(validacion.porMiembro[i] ?? {}).join(' ')}</p>
                )}
              </li>
            ))}
            {equipo.length === 0 && (
              <li className="slot empty">
                <p className="muted small">Busca un Pokémon y añádelo con sus movimientos.</p>
              </li>
            )}
          </ul>

          <div className="row-between wrap panel-foot">
            <span className="small muted">
              {validacion?.general ?? 'Cada Pokémon necesita al menos un movimiento.'}
            </span>
            <button
              type="button"
              className="btn btn-accent"
              onClick={() => void enviar()}
              disabled={enviando || !validacion?.valido || !lider}
            >
              {enviando ? 'Enviando…' : 'Simular combate'}
            </button>
          </div>
        </section>
      )}
    </>
  )
}
