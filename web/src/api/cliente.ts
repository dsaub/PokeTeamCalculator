import type { CuerpoError } from './tipos.ts'

// Toda la API cuelga de /api en el mismo origen. En desarrollo la sirve el mock (VITE_MOCK=1)
// o el proxy de Vite hacia la API en Go; en producción, el mismo servidor que sirve la web.
export const API_BASE = '/api'

const SIN_CONEXION = 'No se pudo conectar con el servidor. Comprueba tu conexión y vuelve a intentarlo.'

/** Error de la API con el mensaje en español listo para mostrar. `status` 0 = fallo de red. */
export class ApiError extends Error {
  readonly status: number
  /** Solo en el 429 de «ya tienes una simulación en curso». */
  readonly idSimulacion?: number

  constructor(status: number, mensaje: string, idSimulacion?: number) {
    super(mensaje)
    this.name = 'ApiError'
    this.status = status
    this.idSimulacion = idSimulacion
  }
}

function mensajePorDefecto(status: number): string {
  if (status === 404) return 'No se ha encontrado lo que buscabas.'
  if (status === 429) return 'Has enviado demasiadas simulaciones seguidas. Espera un momento.'
  if (status >= 500) return 'El servidor no está disponible ahora mismo. Vuelve a intentarlo en un rato.'
  return `El servidor ha respondido con un error (${status}).`
}

async function leerError(res: Response): Promise<ApiError> {
  try {
    const cuerpo = (await res.json()) as Partial<CuerpoError>
    if (typeof cuerpo.error === 'string' && cuerpo.error) {
      return new ApiError(res.status, cuerpo.error, typeof cuerpo.id === 'number' ? cuerpo.id : undefined)
    }
  } catch {
    // Cuerpo vacío o no JSON (p. ej. un proxy intermedio): usamos un mensaje genérico.
  }
  return new ApiError(res.status, mensajePorDefecto(res.status))
}

export interface Respuesta<T> {
  status: number
  datos: T
}

/** Petición a la API. Lanza `ApiError` si no es 2xx o si falla la red. */
export async function pedir<T>(ruta: string, init: RequestInit = {}): Promise<Respuesta<T>> {
  let res: Response
  try {
    res = await fetch(API_BASE + ruta, {
      ...init,
      headers: { Accept: 'application/json', ...init.headers },
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, SIN_CONEXION)
  }
  if (!res.ok) throw await leerError(res)
  const datos = res.status === 204 ? (undefined as T) : ((await res.json()) as T)
  return { status: res.status, datos }
}
