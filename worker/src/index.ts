// Punto de entrada del worker. No sirve HTTP: todo lo que necesita la API pasa
// por Redis. La lógica de arranque y apagado está en arranque.ts.

import { arrancar, crearApagado, ErrorArranque, registrarJson } from "./arranque";

function salirConError(evento: string, mensaje: string): never {
  registrarJson(evento, { mensaje });
  process.exit(1);
}

let worker;
try {
  worker = await arrancar();
} catch (e) {
  if (e instanceof ErrorArranque) salirConError(e.evento, e.message);
  throw e;
}

// Sin Redis no hay trabajo que hacer ni forma de devolverlo: se sale y Docker
// reinicia el worker. Lo que tuviera en curso lo recupera otro cuando caduque
// su latido.
worker.cola.alPerderConexion = (error) => salirConError("redis_perdido", `Se perdió la conexión con Redis: ${error.message}`);

const apagar = crearApagado(worker);
process.on("SIGTERM", () => void apagar("SIGTERM"));
process.on("SIGINT", () => void apagar("SIGINT"));
