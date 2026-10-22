// Arranque y apagado del worker, separados de index.ts para poder probarlos con
// dobles de Redis y del pool. index.ts solo registra las señales y sale.

import { hostname } from "node:os";
import { Cola, ErrorVersion, Procesador, type Registrar } from "./cola";
import { ErrorConfig, leerConfig, type Config } from "./config";
import { Pool, type OpcionesPool } from "./pool";

/** Una línea JSON por evento. Nunca se registra la petición completa. */
export function registrarJson(evento: string, datos: Record<string, unknown> = {}): void {
  console.log(JSON.stringify({ ts: new Date().toISOString(), evento, ...datos }));
}

/** Fallo al arrancar: `evento` es lo que se registra antes de salir con código 1. */
export class ErrorArranque extends Error {
  override name = "ErrorArranque";
  constructor(readonly evento: string, mensaje: string) {
    super(mensaje);
  }
}

export interface DependenciasArranque {
  registrar?: Registrar;
  conectar?: (url: string, worker: string, maxIntentos: number) => Promise<Cola>;
  crearPool?: (opciones: OpcionesPool) => Pool;
  idWorker?: string;
}

export interface WorkerEnMarcha {
  config: Config;
  idWorker: string;
  version: string;
  cola: Cola;
  pool: Pool;
  procesador: Procesador;
}

export function nuevoIdWorker(): string {
  return `${hostname()}-${process.pid}-${crypto.randomUUID().slice(0, 8)}`.slice(-64);
}

/**
 * Lee la configuración, conecta con Redis, comprueba su versión y arranca el
 * procesador. Lanza ErrorArranque si algo de eso falla.
 */
export async function arrancar(
  entorno: Record<string, string | undefined> = process.env,
  {
    registrar = registrarJson,
    conectar = (url, worker, maxIntentos) => Cola.conectar(url, worker, maxIntentos),
    crearPool = (opciones) => new Pool(opciones),
    idWorker = nuevoIdWorker(),
  }: DependenciasArranque = {},
): Promise<WorkerEnMarcha> {
  let config: Config;
  try {
    config = leerConfig(entorno);
  } catch (e) {
    if (e instanceof ErrorConfig) throw new ErrorArranque("config_invalida", e.message);
    throw e;
  }

  let cola: Cola;
  try {
    cola = await conectar(config.redisUrl, idWorker, config.maxIntentos);
  } catch (e) {
    throw new ErrorArranque("redis_no_disponible", `No se pudo conectar con Redis: ${(e as Error).message}`);
  }

  let version: string;
  try {
    version = await cola.comprobarVersion();
  } catch (e) {
    cola.cerrar();
    if (e instanceof ErrorVersion) throw new ErrorArranque("version_redis_invalida", e.message);
    throw new ErrorArranque("redis_no_disponible", `No se pudo consultar Redis: ${(e as Error).message}`);
  }

  const pool = crearPool({ hilos: config.hilos, timeoutMs: config.timeoutTareaMs, registrar });
  const procesador = new Procesador(cola, pool, {
    trabajosSimultaneos: config.trabajosSimultaneos,
    intervaloLatidoMs: config.intervaloLatidoMs,
    latidoCaducadoMs: config.latidoCaducadoMs,
    registrar,
  });

  procesador.iniciar();
  registrar("arranque", {
    worker: idWorker, hilos: pool.total, trabajosSimultaneos: config.trabajosSimultaneos, redis: version,
  });
  return { config, idWorker, version, cola, pool, procesador };
}

/**
 * Devuelve la función de apagado limpio: deja de tomar trabajos, espera a los
 * que están en curso, devuelve a la cola los que no terminaron y se da de
 * baja. Solo actúa la primera vez; al final llama a `salir(0)`.
 */
export function crearApagado(
  { config, cola, pool, procesador }: WorkerEnMarcha,
  registrar: Registrar = registrarJson,
  salir: (codigo: number) => void = (codigo) => process.exit(codigo),
): (senal: string) => Promise<void> {
  let apagando = false;
  return async (senal) => {
    if (apagando) return;
    apagando = true;
    const inicio = performance.now();
    registrar("apagado_iniciado", { senal, trabajosEnCurso: procesador.trabajosEnCurso });

    await procesador.detener();
    const sinTerminar = await procesador.esperarEnCurso(config.esperaApagadoMs);
    procesador.pararTemporizadores();
    for (const id of sinTerminar) {
      try {
        if (await cola.devolver(id)) registrar("trabajo_devuelto", { id });
      } catch (e) {
        registrar("error_devolver", { id, mensaje: (e as Error).message });
      }
    }
    try {
      if (!(await cola.baja())) registrar("baja_aplazada", { motivo: "quedan trabajos en la lista del worker" });
    } catch (e) {
      registrar("error_baja", { mensaje: (e as Error).message });
    }

    // Si quedaban trabajos sin terminar, el pool no espera a sus tareas.
    if (!sinTerminar.length) await pool.cerrar();
    cola.cerrar();
    registrar("apagado", { senal, ms: Math.round(performance.now() - inicio), devueltos: sinTerminar.length });
    salir(0);
  };
}
