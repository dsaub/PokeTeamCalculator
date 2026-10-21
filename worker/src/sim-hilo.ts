// Código que corre dentro de cada hilo del pool: recibe un candidato ya
// validado, simula sus combates y devuelve el resumen.

import { prepararPeticion, simularCandidato } from "./sim/simular";
import type { MensajeHilo, MensajeTarea } from "./pool";

declare var self: Worker;

// Calentamiento: Showdown carga sus datos en el primer combate y cada hilo
// tiene su propio JIT, así que los primeros cientos de combates son varias
// veces más lentos. Mejor pagarlo al crear el hilo que en el primer trabajo.
try {
  for (const gen of ["gen1", "gen4", "gen9"]) {
    const p = prepararPeticion({
      gen, simulaciones: 1,
      rival: { especie: "Geodude", nivel: 12, movimientos: ["Tackle", "Rock Throw", "Defense Curl"] },
      candidatos: [{ especie: "Squirtle", nivel: 12, movimientos: ["Water Gun", "Tackle", "Withdraw"] }],
    });
    simularCandidato(p.gen, p.candidatos[0]!, p.rival, 40, p.opciones);
  }
} catch {
  // Si fallara, el primer trabajo real lo haría igualmente.
}

self.onmessage = (evento: MessageEvent<MensajeTarea>) => {
  const { id, tarea } = evento.data;
  let respuesta: MensajeHilo;
  try {
    const resultado = simularCandidato(tarea.gen, tarea.candidato, tarea.rival, tarea.n, tarea.opciones);
    respuesta = { id, ok: true, resultado };
  } catch (e) {
    const error = e instanceof Error ? e : new Error(String(e));
    respuesta = { id, ok: false, error: `${error.name}: ${error.message}` };
  }
  postMessage(respuesta);
};
