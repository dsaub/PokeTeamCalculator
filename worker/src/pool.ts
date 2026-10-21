// Pool de hilos (Worker nativo de Bun). La unidad de trabajo es un candidato:
// así los candidatos de una misma petición se simulan en paralelo.

import type { OpcionesResueltas, PokemonSet } from "./sim/simular";
import type { ResultadoCandidato } from "./sim/tipos";

export interface Tarea {
  gen: number;
  candidato: PokemonSet;
  rival: PokemonSet;
  n: number;
  opciones: OpcionesResueltas;
}

/** Mensaje del pool al hilo. */
export interface MensajeTarea {
  id: number;
  tarea: Tarea;
}

/** Mensaje del hilo al pool. */
export type MensajeHilo =
  | { id: number; ok: true; resultado: ResultadoCandidato }
  | { id: number; ok: false; error: string };

export class ErrorHilo extends Error {
  override name = "ErrorHilo";
}

export class ErrorTimeout extends Error {
  override name = "ErrorTimeout";
}

export class ErrorPoolCerrado extends Error {
  override name = "ErrorPoolCerrado";
}

export interface OpcionesPool {
  hilos: number;
  timeoutMs: number;
  /** Script de cada hilo. Por defecto, src/sim-hilo.ts (los tests usan otros). */
  script?: string | URL;
  /** Se llama con eventos internos (hilo reemplazado…) para registrarlos. */
  registrar?: (evento: string, datos: Record<string, unknown>) => void;
}

interface Pendiente {
  id: number;
  tarea: Tarea;
  resolver: (r: ResultadoCandidato) => void;
  rechazar: (e: Error) => void;
}

interface Hilo {
  worker: Worker;
  enCurso?: Pendiente;
  temporizador?: Timer;
  /** El pool ya lo dio de baja: sus eventos se ignoran. */
  retirado: boolean;
}

export class Pool {
  private readonly hilos: Hilo[] = [];
  private readonly cola: Pendiente[] = [];
  private readonly script: string;
  private siguienteId = 1;
  private cerrando = false;
  private alVaciarse: (() => void)[] = [];

  constructor(private readonly opciones: OpcionesPool) {
    if (!Number.isInteger(opciones.hilos) || opciones.hilos < 1) throw new Error("El pool necesita al menos un hilo.");
    this.script = String(opciones.script ?? new URL("./sim-hilo.ts", import.meta.url));
    for (let i = 0; i < opciones.hilos; i++) this.hilos.push(this.crearHilo());
  }

  get total(): number {
    return this.hilos.length;
  }

  get ocupados(): number {
    return this.hilos.filter((h) => h.enCurso).length;
  }

  get enCola(): number {
    return this.cola.length;
  }

  /** Simula un candidato en algún hilo libre (o espera turno en la cola). */
  ejecutar(tarea: Tarea): Promise<ResultadoCandidato> {
    if (this.cerrando) return Promise.reject(new ErrorPoolCerrado("El pool se está cerrando."));
    return new Promise((resolver, rechazar) => {
      this.cola.push({ id: this.siguienteId++, tarea, resolver, rechazar });
      this.repartir();
    });
  }

  /** Deja de aceptar tareas, espera a las que quedan y termina los hilos. */
  async cerrar(): Promise<void> {
    this.cerrando = true;
    if (this.ocupados > 0 || this.cola.length > 0) {
      await new Promise<void>((r) => this.alVaciarse.push(r));
    }
    for (const hilo of this.hilos) {
      hilo.retirado = true;
      hilo.worker.terminate();
    }
  }

  private crearHilo(): Hilo {
    const worker = new Worker(this.script);
    const hilo: Hilo = { worker, retirado: false };
    worker.addEventListener("message", (e: MessageEvent<MensajeHilo>) => this.alMensaje(hilo, e.data));
    worker.addEventListener("error", (e: ErrorEvent) => {
      e.preventDefault();
      this.alFallar(hilo, new ErrorHilo(`El hilo lanzó un error: ${e.message}`));
    });
    worker.addEventListener("close", () => this.alFallar(hilo, new ErrorHilo("El hilo terminó inesperadamente.")));
    return hilo;
  }

  private repartir(): void {
    for (const hilo of this.hilos) {
      if (hilo.enCurso) continue;
      const siguiente = this.cola.shift();
      if (!siguiente) break;
      hilo.enCurso = siguiente;
      hilo.temporizador = setTimeout(() => this.alAgotarTiempo(hilo), this.opciones.timeoutMs);
      const mensaje: MensajeTarea = { id: siguiente.id, tarea: siguiente.tarea };
      hilo.worker.postMessage(mensaje);
    }
    this.comprobarVacio();
  }

  private comprobarVacio(): void {
    if (this.cola.length === 0 && this.ocupados === 0 && this.alVaciarse.length) {
      for (const r of this.alVaciarse.splice(0)) r();
    }
  }

  /** Libera el hilo y devuelve su tarea, si la tenía. */
  private liberar(hilo: Hilo): Pendiente | undefined {
    clearTimeout(hilo.temporizador);
    const tarea = hilo.enCurso;
    hilo.enCurso = undefined;
    hilo.temporizador = undefined;
    return tarea;
  }

  private alMensaje(hilo: Hilo, mensaje: MensajeHilo): void {
    if (hilo.retirado || !hilo.enCurso || hilo.enCurso.id !== mensaje.id) return;
    if (!mensaje.ok) {
      // Un error dentro de la simulación es inesperado: no nos fiamos del hilo.
      this.alFallar(hilo, new ErrorHilo(`Error inesperado en el hilo: ${mensaje.error}`));
      return;
    }
    this.liberar(hilo)!.resolver(mensaje.resultado);
    this.repartir();
  }

  private alAgotarTiempo(hilo: Hilo): void {
    if (hilo.retirado) return;
    this.reemplazar(hilo, new ErrorTimeout(`La tarea superó el tiempo máximo de ${this.opciones.timeoutMs} ms.`));
  }

  private alFallar(hilo: Hilo, error: Error): void {
    if (hilo.retirado) return;
    this.reemplazar(hilo, error);
  }

  /** Termina el hilo, rechaza su tarea y pone otro en su lugar. */
  private reemplazar(hilo: Hilo, error: Error): void {
    hilo.retirado = true;
    const tarea = this.liberar(hilo);
    hilo.worker.terminate();
    const i = this.hilos.indexOf(hilo);
    const nuevo = this.crearHilo();
    if (i >= 0) this.hilos[i] = nuevo;
    this.opciones.registrar?.("hilo_reemplazado", { motivo: error.message, tarea: tarea?.id ?? null });
    tarea?.rechazar(error);
    this.repartir();
  }
}
