// Arranque del worker: configuración, MariaDB, pool de hilos, bucle de la
// cola, servidor HTTP interno y apagado limpio.

import { timingSafeEqual } from "node:crypto";
import { hostname } from "node:os";
import { avisarGo, Cola, ErrorVersion, Procesador } from "./cola";
import { ErrorConfig, leerConfig, type Config } from "./config";
import { Pool } from "./pool";

/** Una línea JSON por evento. Nunca se registra la petición completa. */
function registrar(evento: string, datos: Record<string, unknown> = {}): void {
  console.log(JSON.stringify({ ts: new Date().toISOString(), evento, ...datos }));
}

function salirConError(evento: string, mensaje: string): never {
  registrar(evento, { mensaje });
  process.exit(1);
}

/** Compara dos textos sin filtrar por el tiempo cuánto coinciden. */
function mismoToken(recibido: string, esperado: string): boolean {
  const a = new Bun.CryptoHasher("sha256").update(recibido).digest();
  const b = new Bun.CryptoHasher("sha256").update(esperado).digest();
  return timingSafeEqual(a, b);
}

function autorizado(req: Request, config: Config): boolean {
  const cabecera = req.headers.get("authorization") ?? "";
  const m = /^Bearer (.+)$/.exec(cabecera);
  return m !== null && mismoToken(m[1]!, config.tokenInterno);
}

let config: Config;
try {
  config = leerConfig();
} catch (e) {
  if (e instanceof ErrorConfig) salirConError("config_invalida", e.message);
  throw e;
}

const idWorker = `${hostname()}-${process.pid}-${crypto.randomUUID().slice(0, 8)}`.slice(-64);
const bd = Cola.conectar(config.databaseUrl);
const cola = new Cola(bd, idWorker, config.maxIntentos);

let version: string;
try {
  version = await cola.comprobarVersion();
} catch (e) {
  await bd.end().catch(() => {});
  if (e instanceof ErrorVersion) salirConError("version_bd_invalida", e.message);
  salirConError("bd_no_disponible", `No se pudo conectar con MariaDB: ${(e as Error).message}`);
}

const pool = new Pool({ hilos: config.hilos, timeoutMs: config.timeoutTareaMs, registrar });

/** Callbacks a Go en vuelo, para darles una última oportunidad al apagar. */
const avisos = new Set<Promise<unknown>>();

const procesador = new Procesador(cola, pool, {
  trabajosSimultaneos: config.trabajosSimultaneos,
  intervaloSondeoMs: config.intervaloSondeoMs,
  intervaloLatidoMs: config.intervaloLatidoMs,
  latidoCaducadoMs: config.latidoCaducadoMs,
  registrar,
  alTerminar: (id) => {
    const aviso = avisarGo(id, { urlCallback: config.urlCallback, tokenInterno: config.tokenInterno, registrar });
    avisos.add(aviso);
    void aviso.finally(() => avisos.delete(aviso));
  },
});

const servidor = Bun.serve({
  hostname: config.host,
  port: config.puerto,
  routes: {
    "/despertar": {
      POST: (req) => {
        if (!autorizado(req, config)) return Response.json({ error: "no autorizado" }, { status: 401 });
        procesador.despertar();
        return new Response(null, { status: 204 });
      },
    },
    "/salud": {
      GET: async () => {
        const bdResponde = await cola.responde();
        return Response.json({
          worker: idWorker,
          hilos: { total: pool.total, ocupados: pool.ocupados, enCola: pool.enCola },
          trabajosEnCurso: procesador.trabajosEnCurso,
          bd: bdResponde,
        }, { status: bdResponde ? 200 : 503 });
      },
    },
  },
  fetch: () => Response.json({ error: "no encontrado" }, { status: 404 }),
});

procesador.iniciar();
registrar("arranque", {
  worker: idWorker, puerto: servidor.port, hilos: pool.total, trabajosSimultaneos: config.trabajosSimultaneos, mariadb: version,
});

let apagando = false;
async function apagar(senal: string): Promise<void> {
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

  // Si quedaban trabajos sin terminar, el pool no espera a sus tareas.
  if (!sinTerminar.length) await pool.cerrar();
  await Promise.race([Promise.allSettled([...avisos]), Bun.sleep(3000)]);
  servidor.stop(true);
  await bd.end().catch(() => {});
  registrar("apagado", { senal, ms: Math.round(performance.now() - inicio), devueltos: sinTerminar.length });
  process.exit(0);
}

process.on("SIGTERM", () => void apagar("SIGTERM"));
process.on("SIGINT", () => void apagar("SIGINT"));
