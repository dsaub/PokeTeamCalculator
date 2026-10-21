import { API_BASE, pedir } from './cliente.ts'
import type { EstadoSimulacion, PeticionSimulacion, RespuestaCrear } from './tipos.ts'

export async function crearSimulacion(peticion: PeticionSimulacion): Promise<RespuestaCrear> {
  const { datos } = await pedir<RespuestaCrear>('/simulaciones', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(peticion),
  })
  return datos
}

export async function consultarSimulacion(id: number): Promise<EstadoSimulacion> {
  return (await pedir<EstadoSimulacion>(`/simulaciones/${id}`)).datos
}

/** 204 → se canceló; 409 (ya terminada) y 404 llegan como `ApiError`. */
export async function cancelarSimulacion(id: number): Promise<void> {
  await pedir<void>(`/simulaciones/${id}`, { method: 'DELETE' })
}

export function abrirEventos(id: number): EventSource {
  return new EventSource(`${API_BASE}/simulaciones/${id}/eventos`)
}
