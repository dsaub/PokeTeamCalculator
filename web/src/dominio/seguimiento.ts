import { ApiError } from '../api/cliente.ts'
import type { EstadoSimulacion, EventosSse } from '../api/tipos.ts'
import { esTerminal, faseDeEstado, type Accion } from './maquina.ts'

/** Lo que usamos de `EventSource`; los tests pasan uno falso. */
export interface FuenteEventos {
  readonly readyState: number
  addEventListener(tipo: string, fn: (e: Event) => void): void
  close(): void
}

export interface DependenciasSeguimiento {
  abrirEventos: (id: number) => FuenteEventos
  consultar: (id: number) => Promise<EstadoSimulacion>
  programar?: (fn: () => void, ms: number) => unknown
  desprogramar?: (t: unknown) => void
}

export interface OpcionesSeguimiento {
  /** Fallos de conexión seguidos antes de pasar a sondeo. */
  maxFallos?: number
  intervaloSondeoMs?: number
}

const CERRADO = 2 // EventSource.CLOSED
const EVENTOS_FINALES = new Set<keyof EventosSse>(['completado', 'error', 'cancelado'])

/**
 * Sigue una simulación en curso: escucha sus eventos SSE y, si la conexión falla varias veces
 * seguidas (o el navegador la da por perdida), consulta el estado cada pocos segundos hasta que
 * termine. Todo se traduce a acciones de la máquina de estados. Devuelve la función para parar.
 */
export function seguirSimulacion(
  id: number,
  despachar: (a: Accion) => void,
  deps: DependenciasSeguimiento,
  { maxFallos = 3, intervaloSondeoMs = 3000 }: OpcionesSeguimiento = {},
): () => void {
  const programar = deps.programar ?? ((fn, ms) => setTimeout(fn, ms))
  const desprogramar = deps.desprogramar ?? ((t) => clearTimeout(t as ReturnType<typeof setTimeout>))

  let detenido = false
  let fallos = 0
  let fuente: FuenteEventos | null = null
  let temporizador: unknown = null

  function detener() {
    detenido = true
    fuente?.close()
    fuente = null
    if (temporizador !== null) desprogramar(temporizador)
    temporizador = null
  }

  function recibir<K extends keyof EventosSse>(nombre: K, e: Event) {
    if (detenido) return
    let datos: EventosSse[K]
    try {
      datos = JSON.parse((e as MessageEvent<string>).data || '{}')
    } catch {
      return // Evento mal formado: lo ignoramos y seguimos escuchando.
    }
    fallos = 0
    despachar({ tipo: 'evento', evento: { nombre, datos } as Extract<Accion, { tipo: 'evento' }>['evento'] })
    // Tras un evento final el servidor cierra; si no cerramos nosotros, EventSource reconecta.
    if (EVENTOS_FINALES.has(nombre)) detener()
  }

  function sondear() {
    despachar({ tipo: 'conexion', conexion: 'sondeo' })
    const consultar = async () => {
      temporizador = null
      if (detenido) return
      try {
        const estado = await deps.consultar(id)
        if (detenido) return
        despachar({ tipo: 'estado', estado })
        if (esTerminal(faseDeEstado(estado))) return detener()
      } catch (err) {
        if (detenido) return
        if (err instanceof ApiError && err.status === 404) {
          despachar({ tipo: 'consultaFallida', status: 404, mensaje: err.message })
          return detener()
        }
        // Fallo de red u otro error puntual: lo intentamos en el siguiente turno.
      }
      temporizador = programar(consultar, intervaloSondeoMs)
    }
    void consultar()
  }

  function conectar() {
    despachar({ tipo: 'conexion', conexion: 'conectando' })
    const f = deps.abrirEventos(id)
    fuente = f
    f.addEventListener('open', () => {
      if (detenido) return
      fallos = 0
      despachar({ tipo: 'conexion', conexion: 'conectado' })
    })
    for (const nombre of ['posicion', 'procesando', 'completado', 'cancelado'] as const) {
      f.addEventListener(nombre, (e) => recibir(nombre, e))
    }
    f.addEventListener('error', (e) => {
      if (detenido) return
      // El servidor envía `event: error` con datos; EventSource dispara `error` sin datos
      // cuando se corta la conexión.
      const datos = (e as MessageEvent).data
      if (typeof datos === 'string' && datos !== '') return recibir('error', e)
      fallos += 1
      if (fallos >= maxFallos || f.readyState === CERRADO) {
        f.close()
        fuente = null
        sondear()
      } else {
        despachar({ tipo: 'conexion', conexion: 'reconectando' })
      }
    })
  }

  conectar()
  return detener
}
