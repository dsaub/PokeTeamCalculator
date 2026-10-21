// Hilo de pruebas para el pool. La especie del candidato decide qué hace:
//   "colgar"  → bucle infinito (para el timeout)
//   "romper"  → lanza un error sin capturar
//   "fallar"  → responde con un error
//   otra      → espera tarea.n ms y responde con el id de este hilo como "pokemon"
import type { MensajeHilo, MensajeTarea } from "../../src/pool";

declare var self: Worker;
const yo = crypto.randomUUID();

self.onmessage = async (evento: MessageEvent<MensajeTarea>) => {
  const { id, tarea } = evento.data;
  const accion = tarea.candidato.species;
  if (accion === "colgar") while (true) {}
  if (accion === "romper") throw new Error("hilo roto a propósito");
  if (accion === "fallar") {
    postMessage({ id, ok: false, error: "Error: fallo a propósito" } satisfies MensajeHilo);
    return;
  }
  await Bun.sleep(tarea.n);
  postMessage({
    id, ok: true,
    resultado: { pokemon: yo, nivel: 1, victorias: 0, derrotas: 0, empates: 0, winrate: 0, psMedioVictoria: 0, turnosMedio: 0 },
  } satisfies MensajeHilo);
};
