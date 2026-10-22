import type { ResultadoPokemon } from '../api/tipos.ts'

const formatoPorcentaje = new Intl.NumberFormat('es-ES', { style: 'percent', maximumFractionDigits: 0 })
const formatoDecimal = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 1 })

/** 0.754 → «75 %» (con espacio fino como manda la RAE). */
export function porcentaje(proporcion: number): string {
  return formatoPorcentaje.format(proporcion)
}

export function decimal(n: number): string {
  return formatoDecimal.format(n)
}

/** Tiempo de espera aproximado y sin falsa precisión: «unos 20 segundos». */
export function tiempoEstimado(segundos: number): string {
  if (!Number.isFinite(segundos) || segundos < 0) return 'un momento'
  if (segundos < 8) return 'unos segundos'
  if (segundos < 55) return `unos ${Math.max(10, Math.round(segundos / 5) * 5)} segundos`
  if (segundos < 90) return 'alrededor de un minuto'
  return `unos ${Math.round(segundos / 60)} minutos`
}

export function textoPosicion(posicion: number): string {
  return posicion <= 1 ? 'Eres el siguiente en la cola' : `Hay ${posicion - 1} por delante en la cola`
}

export type NombrePara = (especie: string) => string

export function turnos(n: number): string {
  return `${decimal(n)} ${n === 1 ? 'turno' : 'turnos'}`
}

/** Frase que explica la recomendación con los datos del primer puesto. */
export function fraseRecomendacion(r: ResultadoPokemon): string {
  if (r.victorias === 0) return 'Ninguno de tus Pokémon gana ningún combate; es el que mejor resiste, pero conviene cambiar de estrategia.'
  const gana = r.winrate >= 1 ? 'Gana todos los combates' : `Gana el ${porcentaje(r.winrate)} de los combates`
  const ps = `termina con el ${porcentaje(r.psMedioVictoria)} de sus PS`
  const aviso = r.winrate < 0.5 ? ' Aun así, pierde más de lo que gana: es el mejor de tu equipo, no una apuesta segura.' : ''
  return `${gana} y ${ps} de media. Cada combate dura ${turnos(r.turnosMedio)}.${aviso}`
}

/** Mismo porcentaje de victorias, al redondeo que se muestra. */
export function empatados(a: ResultadoPokemon, b: ResultadoPokemon): boolean {
  return porcentaje(a.winrate) === porcentaje(b.winrate)
}

/**
 * Si los dos primeros ganan lo mismo, por qué va primero el primero. El orden lo decide la API;
 * aquí solo se cuenta en qué es mejor. Devuelve null si no hay empate.
 */
export function explicacionEmpate(primero: ResultadoPokemon, segundo: ResultadoPokemon, nombre: NombrePara): string | null {
  if (!empatados(primero, segundo)) return null
  const a = nombre(primero.pokemon)
  const b = nombre(segundo.pokemon)
  const ambos = `${a} y ${b} ganan el ${porcentaje(primero.winrate)} de los combates`
  if (primero.psMedioVictoria > segundo.psMedioVictoria && porcentaje(primero.psMedioVictoria) !== porcentaje(segundo.psMedioVictoria)) {
    return `${ambos}, pero a ${a} le quedan más PS al ganar (${porcentaje(primero.psMedioVictoria)} frente a ${porcentaje(segundo.psMedioVictoria)}).`
  }
  if (primero.turnosMedio < segundo.turnosMedio) {
    return `${ambos}, pero ${a} acaba antes (${turnos(primero.turnosMedio)} frente a ${turnos(segundo.turnosMedio)}).`
  }
  return `${ambos} y con resultados casi idénticos: cualquiera de los dos es buena opción.`
}

/** «Cranidos (Nv. 14)» → { especie, nivel }; null si la API cambia el formato. */
export function leerRival(texto: string): { especie: string; nivel: number } | null {
  const m = /^(.+) \(Nv\. (\d+)\)$/.exec(texto)
  return m ? { especie: m[1], nivel: Number(m[2]) } : null
}

/** Mensaje para mostrar de cualquier error capturado (`ApiError` ya lo trae en español). */
export function mensajeDeError(e: unknown): string {
  if (e instanceof Error) return e.message
  return 'Ha ocurrido un error inesperado.'
}
