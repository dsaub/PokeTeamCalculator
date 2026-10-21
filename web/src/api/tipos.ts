// Formas del contrato con la API (`/api`). Es la única definición: el mock y la interfaz la
// importan de aquí. Los nombres en inglés y formato Showdown (`especie`, `movimientos`,
// `habilidad`, `objeto`…) son los que se envían; `nombre` es solo para mostrar.

export type Gen = `gen${1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9}`

export interface Juego {
  id: string
  nombre: string
  gen: Gen
}

export interface PokemonLider {
  especie: string
  nombre: string
  nivel: number
  habilidad: string | null
  objeto: string | null
  movimientos: string[]
  tipos: string[]
}

export interface Lider {
  id: number
  nombre: string
  ciudad: string
  tipo: string | null
  orden: number
  equipo: PokemonLider[]
}

export interface PokemonResumen {
  especie: string
  nombre: string
  tipos: string[]
}

export interface Habilidad {
  id: string
  nombre: string
}

export interface Movimiento {
  id: string
  nombre: string
  tipo: string
  /** La API devuelve el identificador en minúsculas; null si no aplica. */
  categoria: 'physical' | 'special' | 'status' | null
  /** null en movimientos de estado. */
  potencia: number | null
}

export interface PokemonDetalle extends PokemonResumen {
  habilidades: Habilidad[]
  movimientos: Movimiento[]
}

export const STATS = ['hp', 'atk', 'def', 'spa', 'spd', 'spe'] as const
export type Stat = (typeof STATS)[number]
export type Stats = Partial<Record<Stat, number>>

export interface RivalPeticion {
  especie: string
  nivel: number
  habilidad?: string
  objeto?: string
  movimientos: string[]
}

export interface CandidatoPeticion {
  especie: string
  nivel?: number
  movimientos: string[]
  habilidad?: string
  objeto?: string
  naturaleza?: string
  evs?: Stats
  ivs?: Stats
}

export interface PeticionSimulacion {
  gen: Gen
  rival: RivalPeticion
  candidatos: CandidatoPeticion[]
}

export interface ResultadoPokemon {
  pokemon: string
  nivel: number
  victorias: number
  derrotas: number
  empates: number
  winrate: number
  psMedioVictoria: number
  turnosMedio: number
}

export interface Resultado {
  gen: Gen
  rival: string
  simulaciones: number
  resultados: ResultadoPokemon[]
  ms: number
}

export type EstadoSimulacion =
  | { id: number; estado: 'pendiente'; posicion: number; estimadoSegundos: number }
  | { id: number; estado: 'procesando' }
  | { id: number; estado: 'completado'; resultado: Resultado; cache?: boolean }
  | { id: number; estado: 'error'; error: string }
  | { id: number; estado: 'cancelado' }

/**
 * Respuesta 202 de `POST /api/simulaciones`. Un duplicado del mismo usuario
 * puede llegar ya en `procesando`, sin posición ni estimación.
 */
export interface SimulacionEncolada {
  id: number
  estado: 'pendiente' | 'procesando'
  posicion?: number
  estimadoSegundos?: number
  workerDisponible: boolean
}

/** Respuesta 200 de `POST /api/simulaciones` (caché). */
export interface SimulacionCacheada {
  id: number
  estado: 'completado'
  cache: true
  resultado: Resultado
}

export type RespuestaCrear = SimulacionEncolada | SimulacionCacheada

/** Cuerpo de cualquier error de la API. `id` solo en el 429 de «ya tienes una en curso». */
export interface CuerpoError {
  error: string
  id?: number
}

/** Eventos SSE de `GET /api/simulaciones/{id}/eventos`, por nombre de evento. */
export interface EventosSse {
  posicion: { posicion: number; estimadoSegundos: number }
  procesando: Record<string, never>
  completado: { resultado: Resultado }
  error: { error: string }
  cancelado: Record<string, never>
}
