import { afterEach, describe, expect, test } from "bun:test";
import { Procesador } from "../src/cola";
import type { Peticion } from "../src/sim/tipos";
import ejemplo from "../herramientas/ejemplo.json";
import { ColaFalsa, hasta, PoolFalso, registrador } from "./fixtures/dobles";

// Procesador sin Redis: la cola y el pool son dobles en memoria.

const ERROR_INTERNO = "No se pudo completar la simulación por un error interno. Inténtalo de nuevo más tarde.";
const peticion: Peticion = { ...(structuredClone(ejemplo) as Peticion), simulaciones: 2 };
const texto = (p: unknown = peticion) => JSON.stringify(p);

let abiertos: Procesador[] = [];

afterEach(async () => {
  for (const p of abiertos) {
    await p.detener();
    p.pararTemporizadores();
  }
  abiertos = [];
});

function montar(opciones: { trabajosSimultaneos?: number; intervaloRecuperacionMs?: number; intervaloLatidoMs?: number } = {}) {
  const cola = new ColaFalsa();
  const pool = new PoolFalso();
  const r = registrador();
  const p = new Procesador(cola.comoCola(), pool.comoPool(), {
    trabajosSimultaneos: opciones.trabajosSimultaneos ?? 2,
    intervaloLatidoMs: opciones.intervaloLatidoMs ?? 20,
    latidoCaducadoMs: 1000,
    intervaloRecuperacionMs: opciones.intervaloRecuperacionMs ?? 60_000,
    esperaTomarS: 0.005,
    registrar: r.registrar,
  });
  abiertos.push(p);
  return { cola, pool, p, ...r };
}

describe("procesar un trabajo", () => {
  test("simula cada candidato en el pool y publica el resultado", async () => {
    const { cola, pool, p, registros } = montar();
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 2 });
    p.iniciar();
    await hasta(() => cola.completados.length === 1);
    expect(pool.tareas).toHaveLength(peticion.candidatos.length);
    expect(pool.tareas.every((t) => t.n === 2 && t.gen === 4 && t.rival.species === "Cranidos")).toBe(true);
    const [id, resultado] = cola.completados[0]!;
    expect(id).toBe(1);
    expect(resultado.rival).toBe("Cranidos (Nv. 14)");
    expect(resultado.resultados).toHaveLength(peticion.candidatos.length);
    expect(registros.find((r) => r.evento === "trabajo_tomado")).toEqual({ evento: "trabajo_tomado", id: 1, intento: 2 });
    await hasta(() => registros.some((r) => r.evento === "trabajo_completado"));
    expect(registros.find((r) => r.evento === "trabajo_completado")).toMatchObject({ id: 1, ms: expect.any(Number) });
  });

  test("si completar devuelve false (cancelado o ajeno) lo registra como cancelado", async () => {
    const { cola, p, eventos } = montar();
    cola.completarDevuelve = false;
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => eventos().includes("trabajo_cancelado"));
    expect(eventos()).not.toContain("trabajo_completado");
  });

  test("un trabajo descartado al tomarlo no se simula", async () => {
    const { cola, pool, p, registros } = montar();
    cola.entradas.push({ id: 4, descartado: true });
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "trabajo_cancelado"));
    expect(registros.find((r) => r.evento === "trabajo_cancelado")).toEqual({ evento: "trabajo_cancelado", id: 4 });
    expect(pool.tareas).toHaveLength(0);
  });

  test("JSON roto en la petición → error sin reintento", async () => {
    const { cola, pool, p, registros } = montar();
    cola.entradas.push({ id: 2, peticion: "{roto", intentos: 1 });
    p.iniciar();
    await hasta(() => cola.fallos.length === 1);
    expect(cola.fallos[0]).toEqual([2, "La petición guardada no es un JSON válido.", false]);
    expect(pool.tareas).toHaveLength(0);
    await hasta(() => registros.some((r) => r.evento === "trabajo_error"));
    expect(registros.find((r) => r.evento === "trabajo_error")).toMatchObject({ id: 2, peticion_invalida: true, reintento: false });
  });

  test("petición inválida → error con el mensaje para el jugador", async () => {
    const { cola, p } = montar();
    cola.entradas.push({ id: 3, peticion: texto({ ...peticion, gen: "gen0" }), intentos: 1 });
    p.iniciar();
    await hasta(() => cola.fallos.length === 1);
    const [, mensaje, reintentar] = cola.fallos[0]!;
    expect(mensaje).not.toBe(ERROR_INTERNO);
    expect(reintentar).toBe(false);
  });

  test("un fallo del pool se reintenta con el mensaje genérico", async () => {
    const { cola, pool, p, registros } = montar();
    pool.error = new Error("hilo caído");
    cola.fallarDevuelve = "pendiente";
    cola.entradas.push({ id: 5, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "trabajo_error"));
    expect(cola.fallos[0]).toEqual([5, ERROR_INTERNO, true]);
    expect(registros.find((r) => r.evento === "trabajo_error")).toMatchObject({
      id: 5, mensaje: "hilo caído", peticion_invalida: false, reintento: true,
    });
  });

  test("si fallar devuelve null (cancelado) no registra error", async () => {
    const { cola, pool, p, eventos } = montar();
    pool.error = new Error("hilo caído");
    cola.fallarDevuelve = null;
    cola.entradas.push({ id: 5, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => eventos().includes("trabajo_cancelado"));
    expect(eventos()).not.toContain("trabajo_error");
  });

  test("si fallar lanza, se registra error_interno y el bucle sigue", async () => {
    const { cola, pool, p, registros } = montar();
    pool.error = new Error("hilo caído");
    cola.fallarDevuelve = new Error("redis caído");
    cola.entradas.push({ id: 6, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "error_interno"));
    expect(registros.find((r) => r.evento === "error_interno")).toEqual({ evento: "error_interno", id: 6, mensaje: "redis caído" });
    await hasta(() => p.trabajosEnCurso === 0);
    cola.entradas.push({ id: 7, descartado: true });
    await hasta(() => registros.some((r) => r.evento === "trabajo_cancelado" && r.id === 7));
  });
});

describe("bucle", () => {
  test("iniciar dos veces no arranca dos bucles", async () => {
    const { cola, p } = montar();
    p.iniciar();
    p.iniciar();
    await Bun.sleep(40);
    expect(cola.maxTomando).toBe(1);
  });

  test("un error al tomar se registra y se vuelve a intentar", async () => {
    const { cola, p, registros } = montar();
    cola.entradas.push(new Error("conexión perdida"));
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "error_cola"));
    expect(registros.find((r) => r.evento === "error_cola")).toEqual({ evento: "error_cola", mensaje: "conexión perdida" });
    // detener despierta la espera de 1 s sin esperarla entera.
    const inicio = performance.now();
    await p.detener();
    expect(performance.now() - inicio).toBeLessThan(500);
  });

  test("respeta los trabajos simultáneos y toma el siguiente al terminar uno", async () => {
    const { cola, pool, p } = montar({ trabajosSimultaneos: 1 });
    pool.bloquear = true;
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 1 }, { id: 2, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => p.trabajosEnCurso === 1);
    await Bun.sleep(30);
    expect(p.idsEnCurso).toEqual([1]);
    expect(cola.entradas).toHaveLength(1);
    pool.soltar();
    await hasta(() => p.idsEnCurso[0] === 2);
    pool.bloquear = false;
    pool.soltar();
    await hasta(() => cola.completados.length === 2);
    expect(cola.completados.map(([id]) => id)).toEqual([1, 2]);
  });

  test("un trabajo tomado justo al detener se devuelve sin simular", async () => {
    const { cola, pool, p } = montar();
    let parada: Promise<void> | undefined;
    cola.entradas.push(() => {
      parada = p.detener();
      return { id: 9, peticion: texto(), intentos: 1 };
    });
    p.iniciar();
    await hasta(() => parada !== undefined);
    await parada;
    expect(cola.devueltos).toEqual([9]);
    expect(pool.tareas).toHaveLength(0);
    expect(p.trabajosEnCurso).toBe(0);
  });

  test("detener deja terminar los trabajos en curso", async () => {
    const { cola, pool, p } = montar();
    pool.bloquear = true;
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => p.trabajosEnCurso === 1);
    await p.detener();
    expect(p.trabajosEnCurso).toBe(1);
    pool.soltar();
    expect(await p.esperarEnCurso(1000)).toEqual([]);
    expect(cola.completados).toHaveLength(1);
  });

  test("esperarEnCurso devuelve los que no terminaron a tiempo", async () => {
    const { cola, pool, p } = montar();
    pool.bloquear = true;
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 1 });
    p.iniciar();
    await hasta(() => p.trabajosEnCurso === 1);
    await p.detener();
    const inicio = performance.now();
    expect(await p.esperarEnCurso(30)).toEqual([1]);
    expect(performance.now() - inicio).toBeGreaterThanOrEqual(25);
    pool.soltar();
  });

  test("esperarEnCurso sin trabajos vuelve enseguida", async () => {
    const { p } = montar();
    expect(await p.esperarEnCurso(10_000)).toEqual([]);
    expect(p.trabajosEnCurso).toBe(0);
  });
});

describe("latido y recuperación", () => {
  test("late al arrancar y después cada intervalo, hasta pararTemporizadores", async () => {
    const { cola, p } = montar({ intervaloLatidoMs: 10 });
    p.iniciar();
    await hasta(() => cola.latidos >= 3);
    await p.detener();
    p.pararTemporizadores();
    const latidos = cola.latidos;
    await Bun.sleep(40);
    expect(cola.latidos).toBe(latidos);
  });

  test("un fallo del latido se registra y no para el bucle", async () => {
    const { cola, p, registros } = montar();
    cola.latirLanza = new Error("sin redis");
    cola.entradas.push({ id: 1, descartado: true });
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "trabajo_cancelado"));
    expect(registros.find((r) => r.evento === "error_latido")).toEqual({ evento: "error_latido", mensaje: "sin redis" });
  });

  test("registra los trabajos recuperados y los agotados", async () => {
    const { cola, p, registros } = montar();
    cola.recuperacion = { recuperados: [5, 6], agotados: [7] };
    p.iniciar();
    await hasta(() => registros.filter((r) => r.evento.startsWith("trabajo_")).length === 3);
    expect(registros.filter((r) => r.evento === "trabajo_recuperado").map((r) => r.id)).toEqual([5, 6]);
    expect(registros.find((r) => r.evento === "trabajo_error")).toEqual({
      evento: "trabajo_error", id: 7, mensaje: "intentos agotados tras latidos caducados",
    });
  });

  test("un fallo al recuperar se registra", async () => {
    const { cola, p, registros } = montar();
    cola.recuperacion = new Error("script roto");
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "error_recuperacion"));
    expect(registros.find((r) => r.evento === "error_recuperacion")).toEqual({ evento: "error_recuperacion", mensaje: "script roto" });
  });

  test("un huérfano propio se da por fallido a la segunda pasada", async () => {
    const { cola, pool, p, registros } = montar({ intervaloRecuperacionMs: 15 });
    pool.bloquear = true;
    cola.fallarDevuelve = "pendiente";
    cola.entradas.push({ id: 1, peticion: texto(), intentos: 1 });
    // 1 está en curso (no es huérfano); 9 sí.
    cola.idsPropios = [1, 9];
    p.iniciar();
    await hasta(() => registros.some((r) => r.evento === "trabajo_recuperado"));
    expect(registros.find((r) => r.evento === "trabajo_recuperado")).toEqual({ evento: "trabajo_recuperado", id: 9, propio: true });
    expect(cola.fallos.filter(([id]) => id === 9)[0]).toEqual([9, ERROR_INTERNO, true]);
    expect(cola.fallos.some(([id]) => id === 1)).toBe(false);
    pool.soltar();
  });

  test("un huérfano que desaparece antes de la segunda pasada no se toca", async () => {
    const { cola, p } = montar({ intervaloRecuperacionMs: 15 });
    cola.idsPropios = [9];
    p.iniciar();
    await hasta(() => cola.latidos >= 1);
    cola.idsPropios = [];
    await Bun.sleep(60);
    expect(cola.fallos).toEqual([]);
  });

  test("si fallar el huérfano devuelve null no se registra", async () => {
    const { cola, p, registros } = montar({ intervaloRecuperacionMs: 10 });
    cola.idsPropios = [9];
    cola.fallarDevuelve = null;
    p.iniciar();
    await hasta(() => cola.fallos.length >= 1);
    expect(registros.some((r) => r.evento === "trabajo_recuperado")).toBe(false);
  });
});
