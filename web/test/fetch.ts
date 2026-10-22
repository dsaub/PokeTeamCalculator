// Dobles de `fetch` y `EventSource` para los tests de la capa de API y de las páginas.
import { mock } from 'bun:test'

export interface Llamada {
  url: string
  init: RequestInit
}

type Manejador = (url: string, init: RequestInit) => Response | Promise<Response>

const fetchOriginal = globalThis.fetch
const EventSourceOriginal = globalThis.EventSource

/** Deshace los dobles; lo llama el preload (`test/dom.ts`) después de cada test. */
export function restaurarDobles(): void {
  globalThis.fetch = fetchOriginal
  globalThis.EventSource = EventSourceOriginal
  FuenteFalsa.abiertas = []
}

/** Respuesta JSON con el estado indicado. */
export function json(datos: unknown, status = 200): Response {
  return new Response(JSON.stringify(datos), { status, headers: { 'Content-Type': 'application/json' } })
}

/** Sustituye `fetch` hasta el final del test y devuelve las llamadas recibidas. */
export function falsearFetch(manejador: Manejador): Llamada[] {
  const llamadas: Llamada[] = []
  globalThis.fetch = mock(async (entrada: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(entrada)
    llamadas.push({ url, init })
    return manejador(url, init)
  }) as unknown as typeof fetch
  return llamadas
}

/**
 * Enruta por método y ruta (sin `/api` ni query): `'GET /juegos'`. Lo que no aparece responde
 * 404 con el cuerpo de error de la API.
 */
export function rutas(tabla: Record<string, Manejador | Response | unknown>): Llamada[] {
  return falsearFetch((url, init) => {
    const u = new URL(url, 'http://localhost')
    const clave = `${init.method ?? 'GET'} ${u.pathname.replace(/^\/api/, '')}`
    if (!(clave in tabla)) return json({ error: `sin ruta: ${clave}` }, 404)
    const r = tabla[clave]
    if (typeof r === 'function') return (r as Manejador)(url, init)
    if (r instanceof Response) return r.clone()
    return json(r)
  })
}

/** EventSource falso: registra la URL y deja emitir eventos a mano. */
export class FuenteFalsa {
  static abiertas: FuenteFalsa[] = []
  readyState = 0
  cerrada = false
  private oyentes = new Map<string, ((e: Event) => void)[]>()

  constructor(readonly url: string) {
    FuenteFalsa.abiertas.push(this)
  }
  addEventListener(tipo: string, fn: (e: Event) => void) {
    this.oyentes.set(tipo, [...(this.oyentes.get(tipo) ?? []), fn])
  }
  close() {
    this.cerrada = true
    this.readyState = 2
  }
  emitir(tipo: string, data?: unknown) {
    const e = data === undefined ? new Event(tipo) : new MessageEvent(tipo, { data: JSON.stringify(data) })
    for (const fn of this.oyentes.get(tipo) ?? []) fn(e)
  }
  abrir() {
    this.readyState = 1
    this.emitir('open')
  }
}

export function falsearEventSource(): typeof FuenteFalsa {
  globalThis.EventSource = FuenteFalsa as unknown as typeof EventSource
  return FuenteFalsa
}
