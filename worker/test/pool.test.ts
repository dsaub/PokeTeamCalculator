import { describe, expect, test } from "bun:test";
import { ErrorHilo, ErrorPoolCerrado, ErrorTimeout, Pool, type Tarea } from "../src/pool";
import { prepararPeticion } from "../src/sim/simular";

const SCRIPT_FALSO = new URL("./fixtures/hilo-falso.ts", import.meta.url);

/** Tarea para el hilo falso: `accion` va en la especie y `ms` en n. */
function tareaFalsa(accion: string, ms = 0): Tarea {
  const set = { name: "", species: accion, item: "", ability: "", moves: [], nature: "", gender: "", evs: {}, ivs: {}, level: 1 };
  return { gen: 4, candidato: set as unknown as Tarea["candidato"], rival: set as unknown as Tarea["rival"], n: ms, opciones: { aleatoriedadRival: 0, maxTurnos: 1 } };
}

/**
 * Espera a que la promesa se rechace y devuelve el error. No usamos
 * expect(promesa).rejects porque, mientras espera, Bun no entrega los
 * mensajes de los Worker y la tarea acabaría por timeout.
 */
async function rechazo(promesa: Promise<unknown>): Promise<Error> {
  try {
    await promesa;
  } catch (e) {
    return e as Error;
  }
  throw new Error("se esperaba un rechazo");
}

describe("pool", () => {
  test("simula candidatos de verdad con sim-hilo.ts", async () => {
    const pool = new Pool({ hilos: 2, timeoutMs: 30_000 });
    try {
      const p = prepararPeticion({
        gen: "gen4", simulaciones: 10,
        rival: { especie: "Cranidos", nivel: 14, movimientos: ["Headbutt"] },
        candidatos: [
          { especie: "Monferno", nivel: 16, movimientos: ["Mach Punch"] },
          { especie: "Starly", nivel: 12, movimientos: ["Tackle"] },
        ],
      });
      const resultados = await Promise.all(p.candidatos.map((candidato) =>
        pool.ejecutar({ gen: p.gen, candidato, rival: p.rival, n: p.n, opciones: p.opciones })));
      expect(resultados.map((r) => r.pokemon)).toEqual(["Monferno", "Starly"]);
      for (const r of resultados) expect(r.victorias + r.derrotas + r.empates).toBe(10);
    } finally {
      await pool.cerrar();
    }
  });

  test("reparte las tareas entre los hilos y encola las que sobran", async () => {
    const pool = new Pool({ hilos: 3, timeoutMs: 10_000, script: SCRIPT_FALSO });
    try {
      const inicio = performance.now();
      const promesas = Array.from({ length: 6 }, () => pool.ejecutar(tareaFalsa("dormir", 200)));
      expect(pool.ocupados).toBe(3);
      expect(pool.enCola).toBe(3);
      const resultados = await Promise.all(promesas);
      const ms = performance.now() - inicio;
      expect(new Set(resultados.map((r) => r.pokemon)).size).toBe(3);
      // Dos tandas de 200 ms en paralelo, no seis seguidas.
      expect(ms).toBeLessThan(1000);
      expect(pool.ocupados).toBe(0);
    } finally {
      await pool.cerrar();
    }
  });

  test("respeta el timeout: rechaza la tarea y sustituye el hilo", async () => {
    const eventos: string[] = [];
    const pool = new Pool({ hilos: 1, timeoutMs: 300, script: SCRIPT_FALSO, registrar: (e) => eventos.push(e) });
    try {
      const colgada = pool.ejecutar(tareaFalsa("colgar"));
      const siguiente = pool.ejecutar(tareaFalsa("dormir", 10));
      expect(await rechazo(colgada)).toBeInstanceOf(ErrorTimeout);
      // El hilo nuevo atiende la tarea que esperaba en la cola.
      expect((await siguiente).pokemon).toBeString();
      expect(eventos).toEqual(["hilo_reemplazado"]);
      expect(pool.total).toBe(1);
    } finally {
      await pool.cerrar();
    }
  });

  test("sustituye un hilo que muere o responde con error", async () => {
    const pool = new Pool({ hilos: 1, timeoutMs: 10_000, script: SCRIPT_FALSO });
    try {
      const antes = (await pool.ejecutar(tareaFalsa("dormir"))).pokemon;
      expect(await rechazo(pool.ejecutar(tareaFalsa("romper")))).toBeInstanceOf(ErrorHilo);
      const despues = (await pool.ejecutar(tareaFalsa("dormir"))).pokemon;
      expect(despues).not.toBe(antes);

      expect((await rechazo(pool.ejecutar(tareaFalsa("fallar")))).message).toContain("fallo a propósito");
      expect((await pool.ejecutar(tareaFalsa("dormir"))).pokemon).not.toBe(despues);
    } finally {
      await pool.cerrar();
    }
  });

  test("cerrar() espera a las tareas en curso y deja de aceptar nuevas", async () => {
    const pool = new Pool({ hilos: 2, timeoutMs: 10_000, script: SCRIPT_FALSO });
    let terminadas = 0;
    const promesas = [300, 300, 300].map((ms) => pool.ejecutar(tareaFalsa("dormir", ms)).then(() => terminadas++));
    await Bun.sleep(20);
    await pool.cerrar();
    expect(terminadas).toBe(3);
    await Promise.all(promesas);
    expect(await rechazo(pool.ejecutar(tareaFalsa("dormir")))).toBeInstanceOf(ErrorPoolCerrado);
  });
});
