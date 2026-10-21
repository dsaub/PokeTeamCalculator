import { afterAll, beforeAll, beforeEach, describe, expect, test } from "bun:test";
import type mysql from "mysql2/promise";
import { avisarGo, Cola, ErrorVersion, Procesador, validarVersion, type TrabajoTomado } from "../src/cola";
import { Pool } from "../src/pool";
import type { Peticion, ResultadoCandidato } from "../src/sim/tipos";
import ejemplo from "../herramientas/ejemplo.json";

// Estos tests necesitan una MariaDB de usar y tirar en DATABASE_URL_TEST:
// borran y recrean la tabla simulaciones.
const URL_TEST = process.env.DATABASE_URL_TEST;
if (!URL_TEST) console.warn("⚠ DATABASE_URL_TEST no está definida: se saltan los tests de la cola en MariaDB.");

const MAX_INTENTOS = 3;
const peticionValida: Peticion = { ...(structuredClone(ejemplo) as Peticion), simulaciones: 3 };

function sinRegistro() {}

async function esperarHasta(condicion: () => Promise<boolean>, ms = 10_000): Promise<void> {
  const limite = Date.now() + ms;
  while (!(await condicion())) {
    if (Date.now() > limite) throw new Error("se agotó la espera");
    await Bun.sleep(20);
  }
}

describe.skipIf(!URL_TEST)("cola en MariaDB", () => {
  let bd: mysql.Pool;

  async function encolar(peticion: unknown = peticionValida): Promise<number> {
    const [r] = await bd.query<mysql.ResultSetHeader>(
      "INSERT INTO simulaciones (hash, usuario, peticion) VALUES (?, ?, ?)",
      ["0".repeat(64), "test", JSON.stringify(peticion)],
    );
    return r.insertId;
  }

  async function fila(id: number): Promise<mysql.RowDataPacket> {
    const [filas] = await bd.query<mysql.RowDataPacket[]>("SELECT * FROM simulaciones WHERE id = ?", [id]);
    return filas[0]!;
  }

  beforeAll(async () => {
    bd = Cola.conectar(URL_TEST!);
    await bd.query("DROP TABLE IF EXISTS simulaciones");
    await bd.query(await Bun.file(new URL("../../migraciones/001_simulaciones.sql", import.meta.url)).text());
  });

  beforeEach(async () => {
    await bd.query("DELETE FROM simulaciones");
  });

  afterAll(async () => {
    await bd?.end();
  });

  test("comprueba la versión de MariaDB", async () => {
    expect(await new Cola(bd, "w", MAX_INTENTOS).comprobarVersion()).toMatch(/mariadb/i);
  });

  test("dos workers a la vez nunca cogen el mismo trabajo", async () => {
    const ids = new Set<number>();
    for (let i = 0; i < 40; i++) ids.add(await encolar());
    const colas = ["w1", "w2", "w3"].map((w) => new Cola(Cola.conectar(URL_TEST!), w, MAX_INTENTOS));
    const tomados = await Promise.all(colas.map(async (cola) => {
      const mios: number[] = [];
      for (let t = await cola.tomar(); t; t = await cola.tomar()) mios.push(t.id);
      return mios;
    }));
    await Promise.all(colas.map((c) => c.bd.end()));
    const todos = tomados.flat();
    expect(todos.length).toBe(40);
    expect(new Set(todos)).toEqual(ids);
    // Los tres trabajaron (SKIP LOCKED salta los bloqueados en vez de esperar).
    for (const t of tomados) expect(t.length).toBeGreaterThan(0);
    const [filas] = await bd.query<mysql.RowDataPacket[]>(
      "SELECT COUNT(*) AS n FROM simulaciones WHERE estado = 'procesando' AND intentos = 1 AND iniciado IS NOT NULL AND latido IS NOT NULL");
    expect(Number(filas[0]!.n)).toBe(40);
  });

  test("toma los trabajos por orden de creación", async () => {
    const primero = await encolar();
    const segundo = await encolar();
    const cola = new Cola(bd, "w", MAX_INTENTOS);
    expect((await cola.tomar())!.id).toBe(primero);
    expect((await cola.tomar())!.id).toBe(segundo);
    expect(await cola.tomar()).toBeNull();
  });

  test("recupera los trabajos con el latido caducado y agota los intentos", async () => {
    const cola = new Cola(bd, "w", MAX_INTENTOS);
    const vivo = await encolar();
    const muerto = await encolar();
    const agotado = await encolar();
    for (let i = 0; i < 3; i++) await cola.tomar();
    await bd.query("UPDATE simulaciones SET latido = NOW(3) - INTERVAL 2 MINUTE WHERE id IN (?)", [[muerto, agotado]]);
    await bd.query("UPDATE simulaciones SET intentos = ? WHERE id = ?", [MAX_INTENTOS, agotado]);

    const r = await cola.recuperarCaducados(60_000);
    expect(r).toEqual({ recuperados: [muerto], agotados: [agotado] });
    expect((await fila(vivo)).estado).toBe("procesando");
    expect((await fila(muerto)).estado).toBe("pendiente");
    expect((await fila(agotado)).estado).toBe("error");
    expect((await fila(agotado)).error).toContain("3 veces");
    // El recuperado se puede volver a tomar y gasta otro intento.
    const otraVez = await cola.tomar();
    expect(otraVez!.id).toBe(muerto);
    expect(otraVez!.intentos).toBe(2);
  });

  test("el latido actualiza solo los trabajos propios en proceso", async () => {
    const cola = new Cola(bd, "w", MAX_INTENTOS);
    const id = await encolar();
    await cola.tomar();
    await bd.query("UPDATE simulaciones SET latido = '2000-01-01' WHERE id = ?", [id]);
    await new Cola(bd, "otro", MAX_INTENTOS).latir([id]);
    expect((await fila(id)).latido).toStartWith("2000-01-01");
    await cola.latir([id]);
    expect((await fila(id)).latido).not.toStartWith("2000-01-01");
  });

  test("no sobrescribe un trabajo cancelado mientras se simulaba", async () => {
    const cola = new Cola(bd, "w", MAX_INTENTOS);
    const id = await encolar();
    await cola.tomar();
    await bd.query("UPDATE simulaciones SET estado = 'cancelado' WHERE id = ?", [id]);
    expect(await cola.completar(id, { gen: "gen4", rival: "x", simulaciones: 1, resultados: [], ms: 1 })).toBeFalse();
    expect(await cola.fallar(id, "x", true)).toBeNull();
    expect(await cola.fallar(id, "x", false)).toBeNull();
    const f = await fila(id);
    expect(f.estado).toBe("cancelado");
    expect(f.resultado).toBeNull();
  });

  test("un worker que perdió el trabajo no sobrescribe el de otro", async () => {
    const viejo = new Cola(bd, "w1", MAX_INTENTOS);
    const nuevo = new Cola(bd, "w2", MAX_INTENTOS);
    const id = await encolar();
    await viejo.tomar();
    // Simula que el latido caducó y otro worker lo recuperó.
    await bd.query("UPDATE simulaciones SET estado = 'pendiente', worker = NULL WHERE id = ?", [id]);
    await nuevo.tomar();

    const resultado = { gen: "gen4", rival: "x", simulaciones: 1, resultados: [], ms: 1 };
    expect(await viejo.completar(id, resultado)).toBeFalse();
    expect(await viejo.fallar(id, "tarde", false)).toBeNull();
    expect(await nuevo.completar(id, resultado)).toBeTrue();
    expect((await fila(id)).worker).toBe("w2");
    expect((await fila(id)).estado).toBe("completado");
  });

  test("devolver() al apagar no gasta el intento", async () => {
    const cola = new Cola(bd, "w", MAX_INTENTOS);
    const id = await encolar();
    await cola.tomar();
    expect(await cola.devolver(id)).toBeTrue();
    const f = await fila(id);
    expect(f.estado).toBe("pendiente");
    expect(f.intentos).toBe(0);
  });

  describe("procesador", () => {
    let pool: Pool;
    beforeAll(() => { pool = new Pool({ hilos: 2, timeoutMs: 30_000 }); });
    afterAll(async () => { await pool.cerrar(); });

    function procesador(cola: Cola, poolHilos: Pool, terminados: [number, string][], eventos: string[] = []) {
      return new Procesador(cola, poolHilos, {
        trabajosSimultaneos: 2,
        intervaloSondeoMs: 50,
        intervaloLatidoMs: 100,
        latidoCaducadoMs: 60_000,
        registrar: (e) => eventos.push(e),
        alTerminar: (id, estado) => terminados.push([id, estado]),
      });
    }

    test("completa un trabajo válido y guarda el Resultado", async () => {
      const terminados: [number, string][] = [];
      const p = procesador(new Cola(bd, "w", MAX_INTENTOS), pool, terminados);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => terminados.length > 0);
      await p.detener();
      p.pararTemporizadores();
      const f = await fila(id);
      expect(f.estado).toBe("completado");
      expect(f.terminado).not.toBeNull();
      const resultado = typeof f.resultado === "string" ? JSON.parse(f.resultado) : f.resultado;
      expect(resultado.gen).toBe("gen4");
      expect(resultado.rival).toBe("Cranidos (Nv. 14)");
      expect(resultado.simulaciones).toBe(3);
      expect(resultado.resultados).toHaveLength(6);
      expect(resultado.ms).toBeNumber();
      expect(terminados).toEqual([[id, "completado"]]);
    });

    test("una petición inválida pasa a error sin reintento", async () => {
      const terminados: [number, string][] = [];
      const p = procesador(new Cola(bd, "w", MAX_INTENTOS), pool, terminados);
      const id = await encolar({ ...peticionValida, candidatos: [{ especie: "Sylveon", movimientos: ["Tackle"] }] });
      p.iniciar();
      await esperarHasta(async () => terminados.length > 0);
      await p.detener();
      p.pararTemporizadores();
      const f = await fila(id);
      expect(f.estado).toBe("error");
      expect(f.error).toBe('Candidato 1: "Sylveon" no existe en gen 4.');
      expect(f.intentos).toBe(1);
      expect(terminados).toEqual([[id, "error"]]);
    });

    test("un error inesperado se reintenta hasta agotar los intentos", async () => {
      const terminados: [number, string][] = [];
      const eventos: string[] = [];
      const poolRoto = { ejecutar: () => Promise.reject(new Error("hilo caído")) } as unknown as Pool;
      const p = procesador(new Cola(bd, "w", MAX_INTENTOS), poolRoto, terminados, eventos);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => terminados.length > 0);
      await p.detener();
      p.pararTemporizadores();
      const f = await fila(id);
      expect(f.estado).toBe("error");
      expect(f.intentos).toBe(MAX_INTENTOS);
      expect(f.error).toContain("error interno");
      expect(eventos.filter((e) => e === "trabajo_tomado")).toHaveLength(MAX_INTENTOS);
      expect(terminados).toEqual([[id, "error"]]);
    });

    test("se salta un trabajo que ya estaba cancelado al ir a procesarlo", async () => {
      const terminados: [number, string][] = [];
      const eventos: string[] = [];
      let llamadas = 0;
      const poolEspia = { ejecutar: () => { llamadas++; return Promise.reject(new Error("no debería llamarse")); } } as unknown as Pool;
      // Go cancela justo después de que el worker lo tome.
      class ColaQueCancela extends Cola {
        override async tomar(): Promise<TrabajoTomado | null> {
          const t = await super.tomar();
          if (t) await this.bd.query("UPDATE simulaciones SET estado = 'cancelado' WHERE id = ?", [t.id]);
          return t;
        }
      }
      const p = procesador(new ColaQueCancela(bd, "w", MAX_INTENTOS), poolEspia, terminados, eventos);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => eventos.includes("trabajo_cancelado"));
      await p.detener();
      p.pararTemporizadores();
      expect(llamadas).toBe(0);
      expect((await fila(id)).estado).toBe("cancelado");
      expect(terminados).toEqual([]);
    });

    test("no guarda el resultado si se cancela mientras se simula", async () => {
      const terminados: [number, string][] = [];
      const eventos: string[] = [];
      const candidato: ResultadoCandidato = { pokemon: "x", nivel: 1, victorias: 1, derrotas: 0, empates: 0, winrate: 1, psMedioVictoria: 1, turnosMedio: 1 };
      const poolQueCancela = {
        ejecutar: async () => {
          await bd.query("UPDATE simulaciones SET estado = 'cancelado'");
          return candidato;
        },
      } as unknown as Pool;
      const p = procesador(new Cola(bd, "w", MAX_INTENTOS), poolQueCancela, terminados, eventos);
      const id = await encolar();
      p.iniciar();
      await esperarHasta(async () => eventos.includes("trabajo_cancelado"));
      await p.detener();
      p.pararTemporizadores();
      const f = await fila(id);
      expect(f.estado).toBe("cancelado");
      expect(f.resultado).toBeNull();
      expect(terminados).toEqual([]);
    });

    test("despertar() consulta la cola sin esperar al sondeo", async () => {
      const terminados: [number, string][] = [];
      const p = new Procesador(new Cola(bd, "w", MAX_INTENTOS), pool, {
        trabajosSimultaneos: 2, intervaloSondeoMs: 60_000, intervaloLatidoMs: 1000, latidoCaducadoMs: 60_000,
        registrar: sinRegistro, alTerminar: (id, estado) => terminados.push([id, estado]),
      });
      p.iniciar();
      await Bun.sleep(100);
      const id = await encolar();
      p.despertar();
      await esperarHasta(async () => terminados.length > 0, 5000);
      await p.detener();
      p.pararTemporizadores();
      expect(terminados).toEqual([[id, "completado"]]);
    });
  });
});

/** Servidor TCP que acepta conexiones y nunca envía nada. */
function servidorMudo() {
  return Bun.listen({ hostname: "127.0.0.1", port: 0, socket: { data() {} } });
}

/** Un puerto en el que no escucha nadie. */
function puertoCerrado(): number {
  const s = Bun.listen({ hostname: "127.0.0.1", port: 0, socket: { data() {} } });
  const puerto = s.port;
  s.stop(true);
  return puerto;
}

describe("validarVersion", () => {
  test("rechaza MariaDB anterior a 10.6 y lo que no es MariaDB", () => {
    for (const v of ["10.5.21-MariaDB", "10.5.21-MariaDB-1:10.5.21+maria~ubu2004-log", "10.4.32-MariaDB", "5.5.68-MariaDB",
      "8.0.36", "8.4.0-MySQL", "", "MariaDB"]) {
      expect(() => validarVersion(v)).toThrow(ErrorVersion);
    }
    expect(() => validarVersion("10.5.21-MariaDB")).toThrow('Se necesita MariaDB 10.6 o superior (para SKIP LOCKED) y el servidor es "10.5.21-MariaDB".');
  });

  test("acepta MariaDB 10.6 o superior", () => {
    for (const v of ["10.6.0-MariaDB", "10.6.18-MariaDB-0ubuntu0.22.04.1", "10.11.8-MariaDB-log", "11.8.8-MariaDB", "12.0.2-MariaDB",
      "5.5.5-10.6.18-MariaDB"]) {
      expect(() => validarVersion(v)).not.toThrow();
    }
  });

  test("el prefijo 5.5.5- no oculta una versión antigua", () => {
    expect(() => validarVersion("5.5.5-10.5.21-MariaDB")).toThrow(ErrorVersion);
  });
});

describe("Cola.responde sin MariaDB", () => {
  test("devuelve false si el puerto está cerrado", async () => {
    const bd = Cola.conectar(`mysql://x:y@127.0.0.1:${puertoCerrado()}/z`);
    try {
      expect(await new Cola(bd, "w", MAX_INTENTOS).responde()).toBeFalse();
    } finally {
      await bd.end().catch(() => {});
    }
  });

  test("devuelve false en unos 2 s si el servidor acepta la conexión pero no contesta", async () => {
    const mudo = servidorMudo();
    const bd = Cola.conectar(`mysql://x:y@127.0.0.1:${mudo.port}/z`);
    try {
      const inicio = performance.now();
      expect(await new Cola(bd, "w", MAX_INTENTOS).responde()).toBeFalse();
      expect(performance.now() - inicio).toBeWithin(1900, 2600);
    } finally {
      mudo.stop(true);
      await bd.end().catch(() => {});
    }
  });
});

describe("avisarGo", () => {
  test("el timeout real es de 3 s con un servidor que no responde nunca", async () => {
    const mudo = servidorMudo();
    const eventos: Record<string, unknown>[] = [];
    try {
      const inicio = performance.now();
      // Sin timeoutMs: el valor por defecto. Sin reintentos para que el test no dure 15 s.
      const ok = await avisarGo(9, {
        urlCallback: `http://127.0.0.1:${mudo.port}`, tokenInterno: "t", reintentos: 0,
        registrar: (_, datos) => eventos.push(datos ?? {}),
      });
      const ms = performance.now() - inicio;
      expect(ok).toBeFalse();
      expect(ms).toBeWithin(2950, 3600);
      expect(eventos).toHaveLength(1);
      expect(String(eventos[0]!.mensaje)).toMatch(/time|abort/i);
    } finally {
      mudo.stop(true);
    }
  }, 10_000);

  test("hace POST con el token y reintenta con espera creciente", async () => {
    const llegadas: { auth: string | null; ruta: string; cuerpo: string }[] = [];
    const servidor = Bun.serve({
      port: 0,
      hostname: "127.0.0.1",
      async fetch(req) {
        llegadas.push({ auth: req.headers.get("authorization"), ruta: new URL(req.url).pathname, cuerpo: await req.text() });
        return new Response(null, { status: llegadas.length < 3 ? 503 : 204 });
      },
    });
    try {
      const ok = await avisarGo(42, {
        urlCallback: `http://127.0.0.1:${servidor.port}`, tokenInterno: "secreto", registrar: sinRegistro, esperaBaseMs: 10,
      });
      expect(ok).toBeTrue();
      expect(llegadas).toHaveLength(3);
      expect(llegadas[0]).toEqual({ auth: "Bearer secreto", ruta: "/interno/simulaciones/42/lista", cuerpo: "" });
    } finally {
      servidor.stop(true);
    }
  });

  test("si falla del todo lo registra y no lanza", async () => {
    const eventos: string[] = [];
    const servidor = Bun.serve({ port: 0, hostname: "127.0.0.1", fetch: () => new Response(null, { status: 500 }) });
    try {
      const ok = await avisarGo(7, {
        urlCallback: `http://127.0.0.1:${servidor.port}`, tokenInterno: "t", esperaBaseMs: 1,
        registrar: (e) => eventos.push(e),
      });
      expect(ok).toBeFalse();
      expect(eventos).toEqual(["callback_fallido"]);
    } finally {
      servidor.stop(true);
    }
  });

  test("respeta el timeout", async () => {
    const servidor = Bun.serve({ port: 0, hostname: "127.0.0.1", fetch: async () => { await Bun.sleep(1000); return new Response(); } });
    try {
      const inicio = performance.now();
      const ok = await avisarGo(1, {
        urlCallback: `http://127.0.0.1:${servidor.port}`, tokenInterno: "t", registrar: sinRegistro,
        timeoutMs: 100, reintentos: 1, esperaBaseMs: 1,
      });
      expect(ok).toBeFalse();
      expect(performance.now() - inicio).toBeLessThan(800);
    } finally {
      servidor.stop(true);
    }
  });
});
