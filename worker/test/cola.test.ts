import { afterEach, afterAll, beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { RedisClient } from "bun";
import { Claves, Cola, ErrorVersion, PREFIJO, Procesador, validarVersion } from "../src/cola";
import { Pool } from "../src/pool";
import type { Peticion, ResultadoCandidato } from "../src/sim/tipos";
import ejemplo from "../herramientas/ejemplo.json";

// Estos tests necesitan un Redis en REDIS_URL_TEST. Cada test usa un prefijo
// de claves propio y lo borra al terminar, así que no pisan otros datos.
const URL_TEST = process.env.REDIS_URL_TEST;
if (!URL_TEST) console.warn("⚠ REDIS_URL_TEST no está definida: se saltan los tests de la cola en Redis.");

const MAX_INTENTOS = 3;
const ESPERA_S = 0.05;
const peticionValida: Peticion = { ...(structuredClone(ejemplo) as Peticion), simulaciones: 3 };
const resultadoVacio = { gen: "gen4", rival: "x", simulaciones: 1, resultados: [], ms: 1 };

async function esperarHasta(condicion: () => Promise<boolean>, ms = 10_000): Promise<void> {
  const limite = Date.now() + ms;
  while (!(await condicion())) {
    if (Date.now() > limite) throw new Error("se agotó la espera");
    await Bun.sleep(20);
  }
}

describe.skipIf(!URL_TEST)("cola en Redis", () => {
  let redis: RedisClient;
  let claves: Claves;
  let abiertas: Cola[] = [];
  let siguienteId = 1;

  async function cola(worker = "w"): Promise<Cola> {
    const c = await Cola.conectar(URL_TEST!, worker, MAX_INTENTOS, claves.prefijo);
    abiertas.push(c);
    return c;
  }

  /** Lo mismo que hace la API al crear un trabajo: el hash y LPUSH a la cola. */
  async function encolar(peticion: unknown = peticionValida): Promise<number> {
    const id = siguienteId++;
    await redis.hset(claves.trabajo(id), { peticion: JSON.stringify(peticion), intentos: "0" });
    await redis.lpush(claves.cola, String(id));
    return id;
  }

  /** La cola en orden de salida: el primero es el siguiente que se tomará. */
  async function enCola(): Promise<number[]> {
    return (await redis.lrange(claves.cola, 0, -1)).map(Number).reverse();
  }

  /** Los eventos publicados en el stream de estados. */
  async function estados(): Promise<Record<string, string>[]> {
    const entradas = (await redis.send("XRANGE", [claves.estados, "-", "+"])) as [string, string[]][];
    return entradas.map(([, campos]) => {
      const evento: Record<string, string> = {};
      for (let i = 0; i < campos.length; i += 2) evento[campos[i]!] = campos[i + 1]!;
      return evento;
    });
  }

  async function tomar(c: Cola) {
    const t = await c.tomar(ESPERA_S);
    if (!t || "descartado" in t) throw new Error(`se esperaba un trabajo y llegó ${JSON.stringify(t)}`);
    return t;
  }

  beforeAll(async () => {
    redis = new RedisClient(URL_TEST!);
    await redis.connect();
  });

  beforeEach(() => {
    claves = new Claves(`test:${crypto.randomUUID()}:`);
  });

  afterEach(async () => {
    for (const c of abiertas) c.cerrar();
    abiertas = [];
    let cursor = "0";
    do {
      const [siguiente, encontradas] = (await redis.send("SCAN", [cursor, "MATCH", `${claves.prefijo}*`, "COUNT", "1000"])) as [string, string[]];
      if (encontradas.length) await redis.del(...encontradas);
      cursor = siguiente;
    } while (cursor !== "0");
  });

  afterAll(() => {
    redis?.close();
  });

  test("comprueba la versión de Redis", async () => {
    expect(await (await cola()).comprobarVersion()).toMatch(/^\d+\.\d+\./);
  });

  test("dos workers a la vez nunca cogen el mismo trabajo", async () => {
    const ids = new Set<number>();
    for (let i = 0; i < 40; i++) ids.add(await encolar());
    const colas = await Promise.all(["w1", "w2", "w3"].map((w) => cola(w)));
    const tomados = await Promise.all(colas.map(async (c) => {
      const mios: number[] = [];
      for (let t = await c.tomar(ESPERA_S); t; t = await c.tomar(ESPERA_S)) mios.push(t.id);
      return mios;
    }));
    const todos = tomados.flat();
    expect(todos.length).toBe(40);
    expect(new Set(todos)).toEqual(ids);
    for (const t of tomados) expect(t.length).toBeGreaterThan(0);
    const eventos = await estados();
    expect(eventos.filter((e) => e.tipo === "procesando" && e.intentos === "1")).toHaveLength(40);
  });

  test("toma los trabajos por orden de llegada", async () => {
    const primero = await encolar();
    const segundo = await encolar();
    const c = await cola();
    expect((await tomar(c)).id).toBe(primero);
    expect((await tomar(c)).id).toBe(segundo);
    expect(await c.tomar(ESPERA_S)).toBeNull();
  });

  test("tomar gasta un intento y avisa a la API", async () => {
    const id = await encolar();
    const c = await cola("w1");
    const t = await tomar(c);
    expect(t).toEqual({ id, peticion: JSON.stringify(peticionValida), intentos: 1 });
    expect(await redis.lrange(claves.procesando("w1"), 0, -1)).toEqual([String(id)]);
    expect(await estados()).toEqual([{ id: String(id), tipo: "procesando", intentos: "1", worker: "w1" }]);
  });

  test("un fallo reintentable deja el trabajo detrás de los que ya esperaban", async () => {
    const a = await encolar();
    const b = await encolar();
    const c = await encolar();
    const w = await cola("w1");
    expect((await tomar(w)).id).toBe(a);
    expect(await w.fallar(a, "hilo caído", true)).toBe("pendiente");
    expect(await enCola()).toEqual([b, c, a]);
    expect((await estados()).at(-1)).toEqual({ id: String(a), tipo: "pendiente", motivo: "reintento", intentos: "1", worker: "w1" });
    expect(await redis.llen(claves.procesando("w1"))).toBe(0);
    expect((await tomar(w)).id).toBe(b);
  });

  test("sin intentos, el fallo deja el trabajo en error", async () => {
    const id = await encolar();
    const w = await cola("w1");
    for (let i = 1; i < MAX_INTENTOS; i++) {
      await tomar(w);
      expect(await w.fallar(id, "x", true)).toBe("pendiente");
    }
    expect((await tomar(w)).intentos).toBe(MAX_INTENTOS);
    expect(await w.fallar(id, "No se pudo completar la simulación", true)).toBe("error");
    expect(await enCola()).toEqual([]);
    expect((await estados()).at(-1)).toEqual({
      id: String(id), tipo: "error", intentos: String(MAX_INTENTOS), worker: "w1", error: "No se pudo completar la simulación",
    });
    expect(await redis.hget(claves.trabajo(id), "fin")).toBe("1");
    expect(await redis.pttl(claves.trabajo(id))).toBeGreaterThan(0);
  });

  test("un error de la petición no se reintenta", async () => {
    const id = await encolar();
    const w = await cola();
    await tomar(w);
    expect(await w.fallar(id, "Candidato 1: no existe", false)).toBe("error");
    expect(await enCola()).toEqual([]);
  });

  test("los trabajos de un worker muerto vuelven al final de la cola", async () => {
    const muerto = await cola("w1");
    const vivo = await cola("w2");
    const recuperable = await encolar();
    const agotado = await encolar();
    await muerto.latir();
    await tomar(muerto);
    await tomar(muerto);
    await redis.hset(claves.trabajo(agotado), { intentos: String(MAX_INTENTOS) });
    const b = await encolar();
    const c = await encolar();
    // El latido de w1 se quedó en 1970.
    await redis.zadd(claves.workers, 1, "w1");
    await vivo.latir();

    expect(await vivo.recuperarCaducados(60_000)).toEqual({ recuperados: [recuperable], agotados: [agotado] });
    expect(await enCola()).toEqual([b, c, recuperable]);
    const eventos = await estados();
    expect(eventos).toContainEqual({ id: String(recuperable), tipo: "pendiente", motivo: "caducado", intentos: "1", worker: "w1" });
    expect(eventos).toContainEqual({
      id: String(agotado), tipo: "error", intentos: String(MAX_INTENTOS), worker: "w1",
      error: `La simulación se interrumpió ${MAX_INTENTOS} veces y no se volverá a intentar.`,
    });
    expect(await redis.exists(claves.procesando("w1"))).toBeFalse();
    expect(await redis.zscore(claves.workers, "w1")).toBeNull();
    expect(await redis.zscore(claves.workers, "w2")).not.toBeNull();
  });

  test("un worker no se recupera a sí mismo aunque su latido haya caducado", async () => {
    const w = await cola("w1");
    await encolar();
    await tomar(w);
    await redis.zadd(claves.workers, 1, "w1");
    expect(await w.recuperarCaducados(60_000)).toEqual({ recuperados: [], agotados: [] });
    expect(await redis.llen(claves.procesando("w1"))).toBe(1);
  });

  test("el latido guarda la hora de Redis", async () => {
    const w = await cola("w1");
    await w.latir();
    const [segundos] = (await redis.send("TIME", [])) as [string, string];
    const latido = Number(await redis.zscore(claves.workers, "w1"));
    expect(Math.abs(Number(segundos) * 1000 - latido)).toBeLessThan(5000);
  });

  test("devolver() al apagar lo deja al principio sin gastar el intento", async () => {
    const a = await encolar();
    const b = await encolar();
    const w = await cola("w1");
    await tomar(w);
    expect(await w.devolver(a)).toBeTrue();
    expect(await enCola()).toEqual([a, b]);
    expect(await redis.hget(claves.trabajo(a), "intentos")).toBe("0");
    expect((await estados()).at(-1)).toEqual({ id: String(a), tipo: "pendiente", motivo: "devuelto", intentos: "0", worker: "w1" });
  });

  test("un trabajo cancelado mientras esperaba se descarta sin gastar intento", async () => {
    const id = await encolar();
    await redis.hset(claves.trabajo(id), { cancelado: "1" });
    const w = await cola("w1");
    expect(await w.tomar(ESPERA_S)).toEqual({ id, descartado: true });
    expect(await redis.llen(claves.procesando("w1"))).toBe(0);
    expect(await redis.hget(claves.trabajo(id), "intentos")).toBe("0");
    expect(await estados()).toEqual([]);
  });

  test("no publica nada si se cancela mientras se simulaba", async () => {
    const id = await encolar();
    const w = await cola();
    await tomar(w);
    await redis.hset(claves.trabajo(id), { cancelado: "1" });
    expect(await w.completar(id, resultadoVacio)).toBeFalse();
    expect((await estados()).map((e) => e.tipo)).toEqual(["procesando"]);
    expect(await redis.hget(claves.trabajo(id), "fin")).toBe("1");

    const otro = await encolar();
    await tomar(w);
    await redis.hset(claves.trabajo(otro), { cancelado: "1" });
    expect(await w.fallar(otro, "x", true)).toBeNull();
    expect(await enCola()).toEqual([]);
  });

  test("un worker que perdió el trabajo no publica el resultado", async () => {
    const viejo = await cola("w1");
    const nuevo = await cola("w2");
    const id = await encolar();
    await viejo.latir();
    await tomar(viejo);
    await redis.zadd(claves.workers, 1, "w1");
    await nuevo.recuperarCaducados(60_000);
    expect((await tomar(nuevo)).id).toBe(id);

    expect(await viejo.completar(id, resultadoVacio)).toBeFalse();
    expect(await viejo.fallar(id, "tarde", false)).toBeNull();
    expect(await nuevo.completar(id, resultadoVacio)).toBeTrue();
    const completados = (await estados()).filter((e) => e.tipo === "completado");
    expect(completados).toEqual([{
      id: String(id), tipo: "completado", intentos: "2", worker: "w2", resultado: JSON.stringify(resultadoVacio),
    }]);
  });

  test("baja() solo borra al worker si no le quedan trabajos", async () => {
    const id = await encolar();
    const w = await cola("w1");
    await w.latir();
    await tomar(w);
    expect(await w.baja()).toBeFalse();
    expect(await redis.zscore(claves.workers, "w1")).not.toBeNull();
    await w.completar(id, resultadoVacio);
    expect(await w.baja()).toBeTrue();
    expect(await redis.zscore(claves.workers, "w1")).toBeNull();
  });

  test("propios() lista los trabajos de este worker", async () => {
    const a = await encolar();
    const b = await encolar();
    const w = await cola("w1");
    expect(await w.propios()).toEqual([]);
    await tomar(w);
    await tomar(w);
    expect((await w.propios()).sort()).toEqual([a, b].sort());
    await w.completar(a, resultadoVacio);
    expect(await w.propios()).toEqual([b]);
  });

  test("un trabajo cuyo hash caducó o ya terminó se descarta al tomarlo", async () => {
    const sinHash = siguienteId++;
    await redis.lpush(claves.cola, String(sinHash));
    const terminado = await encolar();
    await redis.hset(claves.trabajo(terminado), { fin: "1" });
    const w = await cola("w1");
    expect(await w.tomar(ESPERA_S)).toEqual({ id: sinHash, descartado: true });
    expect(await w.tomar(ESPERA_S)).toEqual({ id: terminado, descartado: true });
    expect(await w.propios()).toEqual([]);
    expect(await estados()).toEqual([]);
  });

  test("completar marca el fin, deja la lápida y publica el resultado", async () => {
    const id = await encolar();
    const w = await cola("w1");
    await tomar(w);
    expect(await w.completar(id, resultadoVacio)).toBeTrue();
    expect(await redis.hget(claves.trabajo(id), "fin")).toBe("1");
    const ttl = await redis.pttl(claves.trabajo(id));
    expect(ttl).toBeGreaterThan(59 * 60 * 1000);
    expect(ttl).toBeLessThanOrEqual(60 * 60 * 1000);
    expect(await w.propios()).toEqual([]);
  });

  test("completar o devolver un trabajo que no es de este worker no hace nada", async () => {
    const id = await encolar();
    const w = await cola("w1");
    expect(await w.completar(id, resultadoVacio)).toBeFalse();
    expect(await w.devolver(id)).toBeFalse();
    expect(await w.fallar(id, "x", true)).toBeNull();
    expect(await estados()).toEqual([]);
    expect(await enCola()).toEqual([id]);
  });

  test("fallar sin reintento aunque queden intentos deja el trabajo en error", async () => {
    const id = await encolar();
    const w = await cola("w1");
    await tomar(w);
    expect(await w.fallar(id, "Petición inválida", false)).toBe("error");
    expect((await estados()).at(-1)).toEqual({ id: String(id), tipo: "error", intentos: "1", worker: "w1", error: "Petición inválida" });
    expect(await redis.hget(claves.trabajo(id), "fin")).toBe("1");
  });

  test("fallar un trabajo cancelado lo cierra sin publicar", async () => {
    const id = await encolar();
    const w = await cola("w1");
    await tomar(w);
    await redis.hset(claves.trabajo(id), { cancelado: "1" });
    expect(await w.fallar(id, "x", false)).toBeNull();
    expect(await redis.hget(claves.trabajo(id), "fin")).toBe("1");
    expect((await estados()).map((e) => e.tipo)).toEqual(["procesando"]);
  });

  test("devolver no deja los intentos por debajo de 0", async () => {
    const id = await encolar();
    const w = await cola("w1");
    await tomar(w);
    await redis.hset(claves.trabajo(id), { intentos: "0" });
    expect(await w.devolver(id)).toBeTrue();
    expect(await redis.hget(claves.trabajo(id), "intentos")).toBe("0");
    expect((await estados()).at(-1)).toMatchObject({ motivo: "devuelto", intentos: "0" });
  });

  test("devolver un trabajo cancelado o sin hash no lo vuelve a encolar", async () => {
    const cancelado = await encolar();
    const sinHash = await encolar();
    const w = await cola("w1");
    await tomar(w);
    await tomar(w);
    await redis.hset(claves.trabajo(cancelado), { cancelado: "1" });
    await redis.del(claves.trabajo(sinHash));
    expect(await w.devolver(cancelado)).toBeFalse();
    expect(await w.devolver(sinHash)).toBeFalse();
    expect(await enCola()).toEqual([]);
    expect(await w.propios()).toEqual([]);
  });

  test("recuperar salta los trabajos cancelados, terminados o sin hash", async () => {
    const muerto = await cola("w1");
    const vivo = await cola("w2");
    const cancelado = await encolar();
    const terminado = await encolar();
    const sinHash = await encolar();
    for (let i = 0; i < 3; i++) await tomar(muerto);
    await redis.hset(claves.trabajo(cancelado), { cancelado: "1" });
    await redis.hset(claves.trabajo(terminado), { fin: "1" });
    await redis.del(claves.trabajo(sinHash));
    await redis.zadd(claves.workers, 1, "w1");
    expect(await vivo.recuperarCaducados(60_000)).toEqual({ recuperados: [], agotados: [] });
    expect(await enCola()).toEqual([]);
    expect(await redis.exists(claves.procesando("w1"))).toBeFalse();
    expect(await redis.zscore(claves.workers, "w1")).toBeNull();
  });

  test("recuperar no toca a los workers con el latido al día", async () => {
    const a = await cola("w1");
    const b = await cola("w2");
    await encolar();
    await a.latir();
    await tomar(a);
    expect(await b.recuperarCaducados(60_000)).toEqual({ recuperados: [], agotados: [] });
    expect(await a.propios()).toHaveLength(1);
  });

  test("alPerderConexion no salta al cerrar a propósito", async () => {
    const w = await cola("w1");
    let perdida: Error | null = null;
    w.alPerderConexion = (e) => { perdida = e; };
    w.cerrar();
    await Bun.sleep(50);
    expect(perdida).toBeNull();
  });

  describe("procesador", () => {
    let pool: Pool;
    beforeAll(() => { pool = new Pool({ hilos: 2, timeoutMs: 30_000 }); });
    afterAll(async () => { await pool.cerrar(); });

    type Registro = { evento: string } & Record<string, unknown>;

    function procesador(c: Cola, poolHilos: Pool, registros: Registro[] = [], trabajosSimultaneos = 2) {
      return new Procesador(c, poolHilos, {
        trabajosSimultaneos,
        intervaloLatidoMs: 100,
        latidoCaducadoMs: 60_000,
        esperaTomarS: ESPERA_S,
        registrar: (evento, datos) => registros.push({ evento, ...datos }),
      });
    }

    async function parar(p: Procesador) {
      await p.detener();
      p.pararTemporizadores();
    }

    async function finales(): Promise<Record<string, string>[]> {
      return (await estados()).filter((e) => e.tipo === "completado" || e.tipo === "error");
    }

    test("completa un trabajo válido y publica el Resultado", async () => {
      const p = procesador(await cola("w1"), pool);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => (await finales()).length > 0);
      await parar(p);
      const [final] = await finales();
      expect(final).toMatchObject({ id: String(id), tipo: "completado", intentos: "1", worker: "w1" });
      const resultado = JSON.parse(final!.resultado!);
      expect(resultado.gen).toBe("gen4");
      expect(resultado.rival).toBe("Cranidos (Nv. 14)");
      expect(resultado.simulaciones).toBe(3);
      expect(resultado.resultados).toHaveLength(6);
      expect(resultado.ms).toBeNumber();
      expect(await redis.zscore(claves.workers, "w1")).not.toBeNull();
    });

    test("una petición inválida pasa a error sin reintento", async () => {
      const p = procesador(await cola(), pool);
      const id = await encolar({ ...peticionValida, candidatos: [{ especie: "Sylveon", movimientos: ["Tackle"] }] });
      p.iniciar();
      await esperarHasta(async () => (await finales()).length > 0);
      await parar(p);
      expect(await finales()).toEqual([
        { id: String(id), tipo: "error", intentos: "1", worker: "w", error: 'Candidato 1: "Sylveon" no existe en gen 4.' },
      ]);
    });

    test("un error inesperado se reintenta hasta agotar los intentos", async () => {
      const registros: Registro[] = [];
      const poolRoto = { ejecutar: () => Promise.reject(new Error("hilo caído")) } as unknown as Pool;
      const p = procesador(await cola(), poolRoto, registros);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => (await finales()).length > 0);
      await parar(p);
      const [final] = await finales();
      expect(final).toMatchObject({ id: String(id), tipo: "error", intentos: String(MAX_INTENTOS) });
      expect(final!.error).toContain("error interno");
      expect(registros.filter((r) => r.evento === "trabajo_tomado")).toHaveLength(MAX_INTENTOS);
    });

    test("el reintento espera a que terminen los que ya estaban en cola", async () => {
      const registros: Registro[] = [];
      const candidato: ResultadoCandidato = { pokemon: "x", nivel: 1, victorias: 1, derrotas: 0, empates: 0, winrate: 1, psMedioVictoria: 1, turnosMedio: 1 };
      // Falla el primer intento del primer trabajo (una llamada por candidato).
      let llamadas = 0;
      const poolQueFallaUnaVez = {
        ejecutar: () => (++llamadas <= peticionValida.candidatos.length ? Promise.reject(new Error("hilo caído")) : Promise.resolve(candidato)),
      } as unknown as Pool;
      const p = procesador(await cola(), poolQueFallaUnaVez, registros, 1);
      const a = await encolar();
      const b = await encolar();
      p.iniciar();
      await esperarHasta(async () => (await finales()).length === 2);
      await parar(p);
      expect(registros.filter((r) => r.evento === "trabajo_tomado").map((r) => r.id)).toEqual([a, b, a]);
      expect((await finales()).map((e) => [Number(e.id), e.tipo])).toEqual([[b, "completado"], [a, "completado"]]);
    });

    test("se salta un trabajo que ya estaba cancelado al ir a tomarlo", async () => {
      const registros: Registro[] = [];
      let llamadas = 0;
      const poolEspia = { ejecutar: () => { llamadas++; return Promise.reject(new Error("no debería llamarse")); } } as unknown as Pool;
      const p = procesador(await cola(), poolEspia, registros);
      const id = await encolar();
      await redis.hset(claves.trabajo(id), { cancelado: "1" });
      p.iniciar();
      await esperarHasta(async () => registros.some((r) => r.evento === "trabajo_cancelado"));
      await parar(p);
      expect(llamadas).toBe(0);
      expect(await estados()).toEqual([]);
    });

    test("no publica el resultado si se cancela mientras se simula", async () => {
      const registros: Registro[] = [];
      const candidato: ResultadoCandidato = { pokemon: "x", nivel: 1, victorias: 1, derrotas: 0, empates: 0, winrate: 1, psMedioVictoria: 1, turnosMedio: 1 };
      let id = 0;
      const poolQueCancela = {
        ejecutar: async () => {
          await redis.hset(claves.trabajo(id), { cancelado: "1" });
          return candidato;
        },
      } as unknown as Pool;
      const p = procesador(await cola(), poolQueCancela, registros);
      id = await encolar();
      p.iniciar();
      await esperarHasta(async () => registros.some((r) => r.evento === "trabajo_cancelado"));
      await parar(p);
      expect(await finales()).toEqual([]);
    });

    test("toma al instante un trabajo que llega con el bucle esperando", async () => {
      const p = procesador(await cola(), pool);
      p.iniciar();
      await Bun.sleep(100);
      const id = await encolar();
      await esperarHasta(async () => (await finales()).length > 0, 5000);
      await parar(p);
      expect((await finales()).map((e) => [Number(e.id), e.tipo])).toEqual([[id, "completado"]]);
    });
  });
});

describe("validarVersion", () => {
  test("rechaza Redis anterior a 6.2", () => {
    for (const v of ["6.0.20", "6.1.0", "5.0.14", "4.0.0", "", "redis"]) {
      expect(() => validarVersion(v)).toThrow(ErrorVersion);
    }
    expect(() => validarVersion("6.0.20")).toThrow('Se necesita Redis 6.2 o superior (para BLMOVE) y el servidor es "6.0.20".');
  });

  test("acepta Redis 6.2 o superior", () => {
    for (const v of ["6.2.0", "6.2.14", "7.2.4", "8.10.2", "10.0.0"]) {
      expect(() => validarVersion(v)).not.toThrow();
    }
  });
});

describe("Claves", () => {
  test("usa el prefijo sim: por defecto", () => {
    const c = new Claves();
    expect(c.prefijo).toBe(PREFIJO);
    expect([c.cola, c.workers, c.estados, c.procesando("w1"), c.trabajo(7), c.trabajo("8")]).toEqual([
      "sim:cola", "sim:workers", "sim:estados", "sim:procesando:w1", "sim:trabajo:7", "sim:trabajo:8",
    ]);
  });

  test("con otro prefijo", () => {
    const c = new Claves("test:x:");
    expect(c.cola).toBe("test:x:cola");
    expect(c.trabajo(1)).toBe("test:x:trabajo:1");
  });
});

test("ErrorVersion tiene su nombre", () => {
  expect(new ErrorVersion("x").name).toBe("ErrorVersion");
});

