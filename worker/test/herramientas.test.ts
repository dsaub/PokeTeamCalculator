import { afterAll, describe, expect, test } from "bun:test";
import { RedisClient } from "bun";
import { Claves } from "../src/cola";

// Las herramientas son scripts: se lanzan como procesos aparte.

const RAIZ = `${import.meta.dir}/..`;
const PETICION = {
  gen: "gen4", simulaciones: 2,
  rival: { especie: "Cranidos", nivel: 14, movimientos: ["Headbutt"] },
  candidatos: [
    { especie: "Monferno", nivel: 16, movimientos: ["Mach Punch"] },
    { especie: "Starly", nivel: 12, movimientos: ["Tackle"] },
  ],
};

async function lanzar(args: string[], { entrada, env = {} }: { entrada?: string; env?: Record<string, string> } = {}) {
  const proc = Bun.spawn(["bun", ...args], {
    cwd: RAIZ,
    env: { PATH: process.env.PATH ?? "", HOME: process.env.HOME ?? "", ...env },
    stdin: entrada === undefined ? "ignore" : new Blob([entrada]),
    stdout: "pipe",
    stderr: "pipe",
  });
  const [salida, errores, codigo] = await Promise.all([
    new Response(proc.stdout).text(), new Response(proc.stderr).text(), proc.exited,
  ]);
  return { salida, errores, codigo };
}

async function archivo(contenido: unknown): Promise<string> {
  const ruta = `${process.env.TMPDIR ?? "/tmp"}/peticion-${crypto.randomUUID()}.json`;
  await Bun.write(ruta, typeof contenido === "string" ? contenido : JSON.stringify(contenido));
  return ruta;
}

describe("cli.ts", () => {
  test("--json imprime el Resultado", async () => {
    const r = await lanzar(["herramientas/cli.ts", await archivo(PETICION), "--json"]);
    expect(r.codigo).toBe(0);
    const resultado = JSON.parse(r.salida);
    expect(resultado).toMatchObject({ gen: "gen4", rival: "Cranidos (Nv. 14)", simulaciones: 2 });
    expect(resultado.resultados).toHaveLength(2);
  }, 30_000);

  test("lee la petición de stdin y muestra la tabla", async () => {
    const r = await lanzar(["herramientas/cli.ts"], { entrada: JSON.stringify(PETICION) });
    expect(r.codigo).toBe(0);
    expect(r.salida).toContain("gen4 · rival: Cranidos (Nv. 14) · 2 combates por candidato");
    expect(r.salida).toContain("Monferno (Nv. 16)");
    expect(r.salida).toMatch(/ms en total · [\d.]+ ms por combate/);
  }, 30_000);

  test("--textos muestra los textos de exportación sin simular", async () => {
    const r = await lanzar(["herramientas/cli.ts", "--textos", await archivo(PETICION)]);
    expect(r.codigo).toBe(0);
    expect(r.salida).toContain("# Rival\nCranidos\nLevel: 14\n- Headbutt");
    expect(r.salida).toContain("# Candidato 2\nStarly");
  });

  test("JSON inválido → código 1", async () => {
    const r = await lanzar(["herramientas/cli.ts", await archivo("{roto")]);
    expect(r.codigo).toBe(1);
    expect(r.errores).toContain("No se pudo leer la petición");
  });

  test("archivo que no existe → código 1", async () => {
    const r = await lanzar(["herramientas/cli.ts", "/no/existe.json"]);
    expect(r.codigo).toBe(1);
    expect(r.errores).toContain("No se pudo leer la petición");
  });

  test("petición no válida → código 2 con el mensaje", async () => {
    const r = await lanzar(["herramientas/cli.ts", await archivo({ ...PETICION, gen: "gen0" })]);
    expect(r.codigo).toBe(2);
    expect(r.errores).toContain('Petición no válida: La generación debe ser "gen1" … "gen9".');
  });
});

describe("encolar.ts: uso", () => {
  const casos: [string, string[], Record<string, string>][] = [
    ["sin ruta", ["herramientas/encolar.ts"], { REDIS_URL: "redis://127.0.0.1:1" }],
    ["sin REDIS_URL", ["herramientas/encolar.ts", "herramientas/ejemplo.json"], {}],
    ["--id 0", ["herramientas/encolar.ts", "herramientas/ejemplo.json", "--id", "0"], { REDIS_URL: "redis://127.0.0.1:1" }],
    ["--id no numérico", ["herramientas/encolar.ts", "herramientas/ejemplo.json", "--id", "abc"], { REDIS_URL: "redis://127.0.0.1:1" }],
    ["--id sin valor", ["herramientas/encolar.ts", "herramientas/ejemplo.json", "--id"], { REDIS_URL: "redis://127.0.0.1:1" }],
  ];
  for (const [nombre, args, env] of casos) {
    test(`${nombre} → código 1 y uso`, async () => {
      const r = await lanzar(args, { env });
      expect(r.codigo).toBe(1);
      expect(r.errores).toContain("Uso: REDIS_URL=redis://… bun herramientas/encolar.ts");
    });
  }
});

const URL_TEST = process.env.REDIS_URL_TEST;

describe.skipIf(!URL_TEST)("encolar.ts con Redis", () => {
  // encolar.ts siempre usa el prefijo sim:, así que se usa un id al azar muy
  // alto y se borra solo lo que crea el test.
  const claves = new Claves();
  const id = 9_000_000_000 + Math.floor(Math.random() * 1_000_000);
  let redis: RedisClient | undefined;

  afterAll(async () => {
    redis ??= new RedisClient(URL_TEST!);
    await redis.del(claves.trabajo(id));
    await redis.send("LREM", [claves.cola, "0", String(id)]);
    redis.close();
  });

  test("encola el trabajo como la API y rechaza el id repetido", async () => {
    const env = { REDIS_URL: URL_TEST! };
    const r = await lanzar(["herramientas/encolar.ts", "herramientas/ejemplo.json", "--id", String(id)], { env });
    expect(r.errores).toBe("");
    expect(r.codigo).toBe(0);
    expect(r.salida.trim()).toBe(String(id));

    redis = new RedisClient(URL_TEST!);
    const hash = await redis.hgetall(claves.trabajo(id));
    expect(hash.intentos).toBe("0");
    expect(JSON.parse(hash.peticion!)).toEqual(await Bun.file(`${RAIZ}/herramientas/ejemplo.json`).json());
    expect(await redis.ttl(claves.trabajo(id))).toBeGreaterThan(23 * 3600);
    expect(await redis.lrange(claves.cola, 0, -1)).toContain(String(id));

    const otra = await lanzar(["herramientas/encolar.ts", "herramientas/ejemplo.json", "--id", String(id)], { env });
    expect(otra.codigo).toBe(1);
    expect(otra.errores).toContain(`El trabajo ${id} ya existe en Redis.`);
  }, 15_000);
});
