// Dobles de Cola y Pool para probar Procesador y el arranque sin Redis ni hilos.

import type { Cola, TrabajoDescartado, TrabajoTomado } from "../../src/cola";
import type { Pool, Tarea } from "../../src/pool";
import type { Resultado, ResultadoCandidato } from "../../src/sim/tipos";

export type Registro = { evento: string } & Record<string, unknown>;

export function registrador(): { registros: Registro[]; registrar: (evento: string, datos?: Record<string, unknown>) => void; eventos: () => string[] } {
  const registros: Registro[] = [];
  return {
    registros,
    registrar: (evento, datos = {}) => void registros.push({ evento, ...datos }),
    eventos: () => registros.map((r) => r.evento),
  };
}

type Entrada = TrabajoTomado | TrabajoDescartado | Error | (() => TrabajoTomado | TrabajoDescartado | null);

/** Cola en memoria que imita a la de Redis. Los métodos se pueden sustituir en cada test. */
export class ColaFalsa {
  entradas: Entrada[] = [];
  latidos = 0;
  tomando = 0;
  maxTomando = 0;
  version = "8.10.0";
  completados: [number, Resultado][] = [];
  fallos: [number, string, boolean][] = [];
  devueltos: number[] = [];
  bajas = 0;
  cerrada = false;
  alPerderConexion: ((e: Error) => void) | null = null;

  completarDevuelve = true;
  fallarDevuelve: "pendiente" | "error" | null | Error = "error";
  devolverDevuelve: boolean | Error = true;
  bajaDevuelve: boolean | Error = true;
  recuperacion: { recuperados: number[]; agotados: number[] } | Error = { recuperados: [], agotados: [] };
  idsPropios: number[] = [];
  latirLanza: Error | null = null;
  versionLanza: Error | null = null;

  async comprobarVersion(): Promise<string> {
    if (this.versionLanza) throw this.versionLanza;
    return this.version;
  }

  async latir(): Promise<void> {
    this.latidos++;
    if (this.latirLanza) throw this.latirLanza;
  }

  async tomar(esperaS: number): Promise<TrabajoTomado | TrabajoDescartado | null> {
    this.tomando++;
    this.maxTomando = Math.max(this.maxTomando, this.tomando);
    try {
      const e = this.entradas.shift();
      if (e === undefined) {
        // El procesador de arrancar() espera 1 s: aquí basta con ceder un poco.
        await Bun.sleep(Math.min(esperaS * 1000, 20));
        return null;
      }
      if (e instanceof Error) throw e;
      return typeof e === "function" ? e() : e;
    } finally {
      this.tomando--;
    }
  }

  async completar(id: number, resultado: Resultado): Promise<boolean> {
    this.completados.push([id, resultado]);
    return this.completarDevuelve;
  }

  async fallar(id: number, mensaje: string, reintentar: boolean): Promise<"pendiente" | "error" | null> {
    this.fallos.push([id, mensaje, reintentar]);
    if (this.fallarDevuelve instanceof Error) throw this.fallarDevuelve;
    return this.fallarDevuelve;
  }

  async devolver(id: number): Promise<boolean> {
    this.devueltos.push(id);
    if (this.devolverDevuelve instanceof Error) throw this.devolverDevuelve;
    return this.devolverDevuelve;
  }

  async recuperarCaducados(): Promise<{ recuperados: number[]; agotados: number[] }> {
    if (this.recuperacion instanceof Error) throw this.recuperacion;
    const r = this.recuperacion;
    this.recuperacion = { recuperados: [], agotados: [] };
    return r;
  }

  async propios(): Promise<number[]> {
    return this.idsPropios;
  }

  async baja(): Promise<boolean> {
    this.bajas++;
    if (this.bajaDevuelve instanceof Error) throw this.bajaDevuelve;
    return this.bajaDevuelve;
  }

  cerrar(): void {
    this.cerrada = true;
  }

  comoCola(): Cola {
    return this as unknown as Cola;
  }
}

export const candidatoFalso = (pokemon = "x"): ResultadoCandidato => ({
  pokemon, nivel: 1, victorias: 1, derrotas: 0, empates: 0, winrate: 1, psMedioVictoria: 1, turnosMedio: 1,
});

/** Pool que resuelve al momento o, con `bloquear`, cuando se llama a `soltar()`. */
export class PoolFalso {
  tareas: Tarea[] = [];
  total = 1;
  cerrado = false;
  bloquear = false;
  error: Error | null = null;
  private esperando: (() => void)[] = [];

  async ejecutar(tarea: Tarea): Promise<ResultadoCandidato> {
    this.tareas.push(tarea);
    if (this.bloquear) await new Promise<void>((r) => this.esperando.push(r));
    if (this.error) throw this.error;
    return candidatoFalso(tarea.candidato.species);
  }

  soltar(): void {
    for (const r of this.esperando.splice(0)) r();
  }

  async cerrar(): Promise<void> {
    this.cerrado = true;
  }

  comoPool(): Pool {
    return this as unknown as Pool;
  }
}

/** Espera hasta que se cumpla la condición (sondeando cada 5 ms). */
export async function hasta(condicion: () => boolean, ms = 2000): Promise<void> {
  const limite = Date.now() + ms;
  while (!condicion()) {
    if (Date.now() > limite) throw new Error("se agotó la espera");
    await Bun.sleep(5);
  }
}
