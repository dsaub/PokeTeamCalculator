import type { EstadoSimulacion, EventosSse, RespuestaCrear, Resultado } from '../api/tipos.ts'

// Ciclo de vida de una simulación vista desde la interfaz. Es un reducer puro: las acciones
// llegan de la respuesta del POST, de las consultas GET (al cargar y en el sondeo) y de los
// eventos SSE, y todas acaban en el mismo estado.

export type Fase =
  | { tipo: 'cargando' }
  | { tipo: 'pendiente'; posicion: number; estimadoSegundos: number }
  | { tipo: 'procesando' }
  | { tipo: 'completado'; resultado: Resultado; cache: boolean }
  | { tipo: 'error'; mensaje: string }
  | { tipo: 'cancelado'; porUsuario: boolean }
  | { tipo: 'noEncontrada' }
  | { tipo: 'falloCarga'; mensaje: string }

/** Cómo nos enteramos de los cambios mientras la simulación está en curso. */
export type Conexion = 'inactiva' | 'conectando' | 'conectado' | 'reconectando' | 'sondeo'

export type Cancelacion = { tipo: 'no' } | { tipo: 'enviando' } | { tipo: 'fallo'; mensaje: string }

export interface EstadoMaquina {
  fase: Fase
  conexion: Conexion
  workerDisponible: boolean
  cancelacion: Cancelacion
}

type EventoSse = { [K in keyof EventosSse]: { nombre: K; datos: EventosSse[K] } }[keyof EventosSse]

export type Accion =
  | { tipo: 'creada'; respuesta: RespuestaCrear }
  | { tipo: 'estado'; estado: EstadoSimulacion }
  | { tipo: 'evento'; evento: EventoSse }
  | { tipo: 'conexion'; conexion: Exclude<Conexion, 'inactiva'> }
  | { tipo: 'consultaFallida'; status: number; mensaje: string }
  | { tipo: 'reintentar' }
  | { tipo: 'cancelando' }
  | { tipo: 'cancelada' }
  | { tipo: 'cancelacionFallida'; mensaje: string }

export const ESTADO_INICIAL: EstadoMaquina = {
  fase: { tipo: 'cargando' },
  conexion: 'inactiva',
  workerDisponible: true,
  cancelacion: { tipo: 'no' },
}

export function esTerminal(fase: Fase): boolean {
  return fase.tipo === 'completado' || fase.tipo === 'error' || fase.tipo === 'cancelado' || fase.tipo === 'noEncontrada'
}

/** La simulación sigue en curso y hay que escuchar sus eventos. */
export function enCurso(fase: Fase): boolean {
  return fase.tipo === 'pendiente' || fase.tipo === 'procesando'
}

export function faseDeEstado(e: EstadoSimulacion): Fase {
  switch (e.estado) {
    case 'pendiente':
      return { tipo: 'pendiente', posicion: e.posicion, estimadoSegundos: e.estimadoSegundos }
    case 'procesando':
      return { tipo: 'procesando' }
    case 'completado':
      return { tipo: 'completado', resultado: e.resultado, cache: e.cache === true }
    case 'error':
      return { tipo: 'error', mensaje: e.error }
    case 'cancelado':
      return { tipo: 'cancelado', porUsuario: false }
  }
}

function faseDeEvento(ev: EventoSse): Fase {
  switch (ev.nombre) {
    case 'posicion':
      return { tipo: 'pendiente', posicion: ev.datos.posicion, estimadoSegundos: ev.datos.estimadoSegundos }
    case 'procesando':
      return { tipo: 'procesando' }
    case 'completado':
      return { tipo: 'completado', resultado: ev.datos.resultado, cache: false }
    case 'error':
      return { tipo: 'error', mensaje: ev.datos.error }
    case 'cancelado':
      return { tipo: 'cancelado', porUsuario: false }
  }
}

function conFase(estado: EstadoMaquina, fase: Fase): EstadoMaquina {
  if (esTerminal(fase)) return { ...estado, fase, conexion: 'inactiva', cancelacion: { tipo: 'no' } }
  return { ...estado, fase }
}

export function reducir(estado: EstadoMaquina, accion: Accion): EstadoMaquina {
  // Un estado final no cambia: los eventos tardíos (p. ej. un sondeo que vuelve después del
  // `completado` por SSE) se ignoran.
  if (esTerminal(estado.fase)) return estado

  switch (accion.tipo) {
    case 'creada': {
      const r = accion.respuesta
      if (r.estado === 'completado') return conFase(estado, { tipo: 'completado', resultado: r.resultado, cache: true })
      if (r.estado === 'procesando') {
        return conFase({ ...estado, workerDisponible: r.workerDisponible }, { tipo: 'procesando' })
      }
      return conFase(
        { ...estado, workerDisponible: r.workerDisponible },
        { tipo: 'pendiente', posicion: r.posicion ?? 1, estimadoSegundos: r.estimadoSegundos ?? 0 },
      )
    }
    case 'estado':
      return conFase(estado, faseDeEstado(accion.estado))
    case 'evento': {
      const fase = faseDeEvento(accion.evento)
      // Si el jugador ha pedido cancelar, el `cancelado` es suyo.
      if (fase.tipo === 'cancelado' && estado.cancelacion.tipo === 'enviando') fase.porUsuario = true
      return conFase(estado, fase)
    }
    case 'conexion':
      if (!enCurso(estado.fase) && estado.fase.tipo !== 'cargando') return estado
      return { ...estado, conexion: accion.conexion }
    case 'consultaFallida':
      if (accion.status === 404) return conFase(estado, { tipo: 'noEncontrada' })
      // Durante el seguimiento un fallo puntual no cambia la fase: se vuelve a intentar.
      if (enCurso(estado.fase)) return estado
      return { ...estado, fase: { tipo: 'falloCarga', mensaje: accion.mensaje } }
    case 'reintentar':
      return estado.fase.tipo === 'falloCarga' ? { ...estado, fase: { tipo: 'cargando' } } : estado
    case 'cancelando':
      return enCurso(estado.fase) ? { ...estado, cancelacion: { tipo: 'enviando' } } : estado
    case 'cancelada':
      return conFase(estado, { tipo: 'cancelado', porUsuario: true })
    case 'cancelacionFallida':
      return { ...estado, cancelacion: { tipo: 'fallo', mensaje: accion.mensaje } }
  }
}
