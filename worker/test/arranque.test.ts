import { describe, expect, spyOn, test } from "bun:test";
import { arrancar, crearApagado, ErrorArranque, nuevoIdWorker, registrarJson, type WorkerEnMarcha } from "../src/arranque";
import { ErrorVersion } from "../src/cola";
import { ColaFalsa, hasta, PoolFalso, registrador } from "./fixtures/dobles";

const entorno = { REDIS_URL: "redis://127.0.0.1:6379/0", HILOS: "2", ESPERA_APAGADO_MS: "50", INTERVALO_LATIDO_MS: "100" };

async function errorDe(p: Promise<unknown>): Promise<ErrorArranque> {
  try {
    await p;
  } catch (e) {
    expect(e).toBeInstanceOf(ErrorArranque);
    return e as ErrorArranque;
  }
  throw new Error("se esperaba un ErrorArranque");
}

function dobles() {
  const cola = new ColaFalsa();
  const pool = new PoolFalso();
  const r = registrador();
  const conexiones: unknown[][] = [];
  const opcionesPool: unknown[] = [];
  return {
    cola,
    pool,
    conexiones,
    opcionesPool,
    ...r,
    deps: {
      registrar: r.registrar,
      idWorker: "w-test",
      conectar: async (...args: unknown[]) => {
        conexiones.push(args);
        return cola.comoCola();
      },
      crearPool: (o: unknown) => {
        opcionesPool.push(o);
        return pool.comoPool();
      },
    },
  };
}

describe("registrarJson", () => {
  test("una línea JSON con la hora, el evento y los datos", () => {
    const log = spyOn(console, "log").mockImplementation(() => {});
    try {
      registrarJson("arranque", { hilos: 2 });
      registrarJson("sin_datos");
      const linea = JSON.parse(log.mock.calls[0]![0] as string);
      expect(linea).toEqual({ ts: expect.any(String), evento: "arranque", hilos: 2 });
      expect(Number.isNaN(Date.parse(linea.ts))).toBe(false);
      expect(JSON.parse(log.mock.calls[1]![0] as string)).toEqual({ ts: expect.any(String), evento: "sin_datos" });
    } finally {
      log.mockRestore();
    }
  });
});

test("nuevoIdWorker: único y de como mucho 64 caracteres", () => {
  const a = nuevoIdWorker();
  expect(a.length).toBeLessThanOrEqual(64);
  expect(a).toContain(String(process.pid));
  expect(nuevoIdWorker()).not.toBe(a);
});

describe("arrancar", () => {
  test("configuración inválida → config_invalida, sin conectar", async () => {
    const d = dobles();
    const e = await errorDe(arrancar({}, d.deps));
    expect(e.evento).toBe("config_invalida");
    expect(e.message).toContain("REDIS_URL es obligatoria.");
    expect(d.conexiones).toHaveLength(0);
  });

  test("Redis inalcanzable → redis_no_disponible", async () => {
    const d = dobles();
    const e = await errorDe(arrancar(entorno, { ...d.deps, conectar: () => Promise.reject(new Error("ECONNREFUSED")) }));
    expect(e.evento).toBe("redis_no_disponible");
    expect(e.message).toBe("No se pudo conectar con Redis: ECONNREFUSED");
  });

  test("Redis antiguo → version_redis_invalida y cierra la conexión", async () => {
    const d = dobles();
    d.cola.versionLanza = new ErrorVersion("Se necesita Redis 6.2");
    const e = await errorDe(arrancar(entorno, d.deps));
    expect(e.evento).toBe("version_redis_invalida");
    expect(d.cola.cerrada).toBe(true);
  });

  test("fallo al consultar la versión → redis_no_disponible y cierra", async () => {
    const d = dobles();
    d.cola.versionLanza = new Error("timeout");
    const e = await errorDe(arrancar(entorno, d.deps));
    expect(e.evento).toBe("redis_no_disponible");
    expect(e.message).toBe("No se pudo consultar Redis: timeout");
    expect(d.cola.cerrada).toBe(true);
  });

  test("otro error al leer la configuración se propaga tal cual", async () => {
    const d = dobles();
    const entornoRoto = new Proxy({}, { get: () => { throw new TypeError("entorno roto"); } });
    await expect(arrancar(entornoRoto, d.deps)).rejects.toThrow(TypeError);
  });

  test("conecta, crea el pool, arranca el procesador y lo registra", async () => {
    const d = dobles();
    const w = await arrancar(entorno, d.deps);
    try {
      expect(d.conexiones).toEqual([["redis://127.0.0.1:6379/0", "w-test", 3]]);
      expect(d.opcionesPool).toEqual([{ hilos: 2, timeoutMs: 60000, registrar: d.registrar }]);
      expect(w).toMatchObject({ idWorker: "w-test", version: "8.10.0" });
      expect(d.registros.find((r) => r.evento === "arranque")).toEqual({
        evento: "arranque", worker: "w-test", hilos: 1, trabajosSimultaneos: 2, redis: "8.10.0",
      });
      await hasta(() => d.cola.latidos > 0);
    } finally {
      await w.procesador.detener();
      w.procesador.pararTemporizadores();
    }
  });
});

describe("apagado", () => {
  async function enMarcha() {
    const d = dobles();
    const w: WorkerEnMarcha = await arrancar(entorno, d.deps);
    const salidas: number[] = [];
    const apagar = crearApagado(w, d.registrar, (c) => void salidas.push(c));
    return { ...d, w, apagar, salidas };
  }

  test("sin trabajos: se da de baja, cierra el pool y la cola y sale con 0", async () => {
    const d = await enMarcha();
    await d.apagar("SIGTERM");
    expect(d.eventos()).toContain("apagado_iniciado");
    expect(d.cola.bajas).toBe(1);
    expect(d.pool.cerrado).toBe(true);
    expect(d.cola.cerrada).toBe(true);
    expect(d.salidas).toEqual([0]);
    expect(d.registros.at(-1)).toMatchObject({ evento: "apagado", senal: "SIGTERM", devueltos: 0 });
  });

  test("solo actúa la primera vez", async () => {
    const d = await enMarcha();
    await Promise.all([d.apagar("SIGTERM"), d.apagar("SIGINT")]);
    await d.apagar("SIGINT");
    expect(d.salidas).toEqual([0]);
    expect(d.eventos().filter((e) => e === "apagado")).toHaveLength(1);
  });

  test("devuelve los trabajos que no terminan a tiempo y no espera al pool", async () => {
    const d = await enMarcha();
    d.pool.bloquear = true;
    d.cola.bajaDevuelve = false;
    d.cola.entradas.push({ id: 3, peticion: JSON.stringify((await import("../herramientas/ejemplo.json")).default), intentos: 1 });
    await hasta(() => d.w.procesador.trabajosEnCurso === 1);
    await d.apagar("SIGINT");
    expect(d.cola.devueltos).toEqual([3]);
    expect(d.registros).toContainEqual({ evento: "trabajo_devuelto", id: 3 });
    expect(d.registros).toContainEqual({ evento: "baja_aplazada", motivo: "quedan trabajos en la lista del worker" });
    expect(d.pool.cerrado).toBe(false);
    expect(d.registros.at(-1)).toMatchObject({ evento: "apagado", devueltos: 1 });
    d.pool.soltar();
  });

  test("errores al devolver o al darse de baja se registran y el apagado sigue", async () => {
    const d = await enMarcha();
    d.pool.bloquear = true;
    d.cola.devolverDevuelve = new Error("devolver falló");
    d.cola.bajaDevuelve = new Error("baja falló");
    d.cola.entradas.push({ id: 4, peticion: JSON.stringify((await import("../herramientas/ejemplo.json")).default), intentos: 1 });
    await hasta(() => d.w.procesador.trabajosEnCurso === 1);
    await d.apagar("SIGTERM");
    expect(d.registros).toContainEqual({ evento: "error_devolver", id: 4, mensaje: "devolver falló" });
    expect(d.registros).toContainEqual({ evento: "error_baja", mensaje: "baja falló" });
    expect(d.salidas).toEqual([0]);
    d.pool.soltar();
  });

  test("un trabajo que la cola ya no devuelve no se registra como devuelto", async () => {
    const d = await enMarcha();
    d.pool.bloquear = true;
    d.cola.devolverDevuelve = false;
    d.cola.entradas.push({ id: 5, peticion: JSON.stringify((await import("../herramientas/ejemplo.json")).default), intentos: 1 });
    await hasta(() => d.w.procesador.trabajosEnCurso === 1);
    await d.apagar("SIGTERM");
    expect(d.eventos()).not.toContain("trabajo_devuelto");
    d.pool.soltar();
  });
});

describe("index.ts", () => {
  test("con configuración inválida registra config_invalida y sale con 1", async () => {
    const proc = Bun.spawn(["bun", "src/index.ts"], {
      cwd: `${import.meta.dir}/..`,
      env: { PATH: process.env.PATH ?? "", HOME: process.env.HOME ?? "" },
      stdout: "pipe",
      stderr: "pipe",
    });
    const salida = await new Response(proc.stdout).text();
    expect(await proc.exited).toBe(1);
    const linea = JSON.parse(salida.trim().split("\n").at(-1)!);
    expect(linea).toMatchObject({ evento: "config_invalida" });
    expect(linea.mensaje).toContain("REDIS_URL es obligatoria.");
  });
});
