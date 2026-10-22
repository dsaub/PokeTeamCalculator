// Cola de trabajos en Redis. La API (Go) encola los trabajos y es la única que
// toca MariaDB; el worker los toma de Redis y publica cada cambio de estado en
// el stream `estados`, que la API aplica. Contrato: docs/cola-redis.md.

import { RedisClient } from "bun";
import type { Pool as PoolHilos } from "./pool";
import { construirResultado, prepararPeticion } from "./sim/simular";
import { ErrorPeticion, type Peticion, type Resultado } from "./sim/tipos";

export type Registrar = (evento: string, datos?: Record<string, unknown>) => void;

export interface TrabajoTomado {
  id: number;
  /** La petición tal como la guardó la API, en texto JSON. */
  peticion: string;
  intentos: number;
}

/** Un trabajo que llegó cancelado (o ya terminado) y se descartó sin simular. */
export interface TrabajoDescartado {
  id: number;
  descartado: true;
}

export class ErrorVersion extends Error {
  override name = "ErrorVersion";
}

/** Lanza ErrorVersion si `version` (redis_version de INFO) es anterior a 6.2. */
export function validarVersion(version: string): void {
  const m = /^(\d+)\.(\d+)\./.exec(version);
  const mayor = Number(m?.[1] ?? 0), menor = Number(m?.[2] ?? 0);
  if (!m || mayor < 6 || (mayor === 6 && menor < 2)) {
    throw new ErrorVersion(`Se necesita Redis 6.2 o superior (para BLMOVE) y el servidor es "${version}".`);
  }
}

/** Prefijo de todas las claves. Los tests usan otro para no pisar datos. */
export const PREFIJO = "sim:";

/** Cuánto se guarda el hash de un trabajo terminado, como lápida. */
const TTL_LAPIDA_MS = 60 * 60 * 1000;

/** Nombres de las claves. Deben coincidir con api/endpoints/simulaciones/cola.go. */
export class Claves {
  constructor(readonly prefijo = PREFIJO) {}

  get cola(): string {
    return `${this.prefijo}cola`;
  }

  get workers(): string {
    return `${this.prefijo}workers`;
  }

  get estados(): string {
    return `${this.prefijo}estados`;
  }

  procesando(worker: string): string {
    return `${this.prefijo}procesando:${worker}`;
  }

  trabajo(id: number | string): string {
    return `${this.prefijo}trabajo:${id}`;
  }
}

// Scripts Lua. Cada uno se ejecuta de forma atómica en Redis. El worker toma
// por la derecha de `cola`: LPUSH deja un trabajo al final y RPUSH, al
// principio.

/** KEYS: workers. ARGV: worker. Guarda el latido con la hora del servidor. */
const LATIR = `
local t = redis.call('TIME')
redis.call('ZADD', KEYS[1], t[1] * 1000 + math.floor(t[2] / 1000), ARGV[1])
return 1`;

/**
 * KEYS: trabajo, procesando, estados. ARGV: id, worker.
 * Lo llama tomar() justo después del BLMOVE. Descarta el trabajo si se canceló
 * (o terminó) mientras esperaba; si no, gasta un intento y avisa a la API.
 */
const EMPEZAR = `
if redis.call('EXISTS', KEYS[1]) == 0 or redis.call('HGET', KEYS[1], 'cancelado') or redis.call('HGET', KEYS[1], 'fin') then
  redis.call('LREM', KEYS[2], 0, ARGV[1])
  return false
end
local intentos = redis.call('HINCRBY', KEYS[1], 'intentos', 1)
redis.call('XADD', KEYS[3], '*', 'id', ARGV[1], 'tipo', 'procesando', 'intentos', intentos, 'worker', ARGV[2])
return {redis.call('HGET', KEYS[1], 'peticion'), intentos}`;

/**
 * KEYS: trabajo, procesando, estados. ARGV: id, worker, resultado, ttl lápida.
 * Solo publica si el trabajo sigue en la lista de este worker y no se canceló.
 */
const COMPLETAR = `
if redis.call('LREM', KEYS[2], 0, ARGV[1]) == 0 then return 0 end
local cancelado = redis.call('HGET', KEYS[1], 'cancelado')
redis.call('HSET', KEYS[1], 'fin', '1')
redis.call('PEXPIRE', KEYS[1], ARGV[4])
if cancelado then return 0 end
local intentos = redis.call('HGET', KEYS[1], 'intentos') or '0'
redis.call('XADD', KEYS[3], '*', 'id', ARGV[1], 'tipo', 'completado', 'intentos', intentos, 'worker', ARGV[2], 'resultado', ARGV[3])
return 1`;

/**
 * KEYS: trabajo, procesando, cola, estados.
 * ARGV: id, worker, mensaje, reintentar ("1"/"0"), máximo de intentos, ttl lápida.
 * Si se puede reintentar, el trabajo vuelve al FINAL de la cola.
 */
const FALLAR = `
if redis.call('LREM', KEYS[2], 0, ARGV[1]) == 0 then return false end
if redis.call('HGET', KEYS[1], 'cancelado') then
  redis.call('HSET', KEYS[1], 'fin', '1')
  redis.call('PEXPIRE', KEYS[1], ARGV[6])
  return false
end
local intentos = tonumber(redis.call('HGET', KEYS[1], 'intentos') or '0')
if ARGV[4] == '1' and intentos < tonumber(ARGV[5]) then
  redis.call('LPUSH', KEYS[3], ARGV[1])
  redis.call('XADD', KEYS[4], '*', 'id', ARGV[1], 'tipo', 'pendiente', 'motivo', 'reintento', 'intentos', intentos, 'worker', ARGV[2])
  return 'pendiente'
end
redis.call('HSET', KEYS[1], 'fin', '1')
redis.call('PEXPIRE', KEYS[1], ARGV[6])
redis.call('XADD', KEYS[4], '*', 'id', ARGV[1], 'tipo', 'error', 'intentos', intentos, 'worker', ARGV[2], 'error', ARGV[3])
return 'error'`;

/**
 * KEYS: trabajo, procesando, cola, estados. ARGV: id, worker.
 * Apagado limpio: el trabajo no ha fallado, así que vuelve al PRINCIPIO de la
 * cola y recupera el intento.
 */
const DEVOLVER = `
if redis.call('LREM', KEYS[2], 0, ARGV[1]) == 0 then return 0 end
if redis.call('EXISTS', KEYS[1]) == 0 or redis.call('HGET', KEYS[1], 'cancelado') then return 0 end
local intentos = redis.call('HINCRBY', KEYS[1], 'intentos', -1)
if intentos < 0 then
  redis.call('HSET', KEYS[1], 'intentos', 0)
  intentos = 0
end
redis.call('RPUSH', KEYS[3], ARGV[1])
redis.call('XADD', KEYS[4], '*', 'id', ARGV[1], 'tipo', 'pendiente', 'motivo', 'devuelto', 'intentos', intentos, 'worker', ARGV[2])
return 1`;

/**
 * KEYS: workers, cola, estados.
 * ARGV: caducado (ms), máximo de intentos, este worker, prefijo, ttl lápida, mensaje de agotado.
 * Vacía la lista de cada worker con el latido caducado (menos este): sus
 * trabajos vuelven al FINAL de la cola o, sin intentos, pasan a error. Usa
 * claves calculadas aquí dentro, así que necesita Redis sin Cluster.
 */
const RECUPERAR = `
local t = redis.call('TIME')
local limite = t[1] * 1000 + math.floor(t[2] / 1000) - tonumber(ARGV[1])
local recuperados, agotados = {}, {}
for _, w in ipairs(redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', '(' .. limite)) do
  if w ~= ARGV[3] then
    local lista = ARGV[4] .. 'procesando:' .. w
    local id = redis.call('RPOP', lista)
    while id do
      local h = ARGV[4] .. 'trabajo:' .. id
      if redis.call('EXISTS', h) == 1 and not redis.call('HGET', h, 'cancelado') and not redis.call('HGET', h, 'fin') then
        local intentos = tonumber(redis.call('HGET', h, 'intentos') or '0')
        if intentos < tonumber(ARGV[2]) then
          redis.call('LPUSH', KEYS[2], id)
          redis.call('XADD', KEYS[3], '*', 'id', id, 'tipo', 'pendiente', 'motivo', 'caducado', 'intentos', intentos, 'worker', w)
          recuperados[#recuperados + 1] = id
        else
          redis.call('HSET', h, 'fin', '1')
          redis.call('PEXPIRE', h, ARGV[5])
          redis.call('XADD', KEYS[3], '*', 'id', id, 'tipo', 'error', 'intentos', intentos, 'worker', w, 'error', ARGV[6])
          agotados[#agotados + 1] = id
        end
      end
      id = redis.call('RPOP', lista)
    end
    redis.call('ZREM', KEYS[1], w)
  end
end
return {recuperados, agotados}`;

/**
 * KEYS: workers, procesando. ARGV: worker.
 * Apagado: se borra de los workers vivos solo si no le queda ningún trabajo;
 * si le queda alguno, el latido caduca y otro worker lo recupera.
 */
const BAJA = `
if redis.call('LLEN', KEYS[2]) > 0 then return 0 end
redis.call('ZREM', KEYS[1], ARGV[1])
return 1`;

/** Acceso a Redis. Cada operación sobre un trabajo es un script atómico. */
export class Cola {
  readonly claves: Claves;
  /** Se llama si Redis se pierde del todo (el cliente agotó sus reconexiones). */
  alPerderConexion: ((error: Error) => void) | null = null;
  private cerrada = false;

  private constructor(
    readonly redis: RedisClient,
    /** Conexión aparte para BLMOVE: un comando bloqueante ocupa la suya. */
    private readonly bloqueo: RedisClient,
    readonly worker: string,
    private readonly maxIntentos: number,
    prefijo: string,
  ) {
    this.claves = new Claves(prefijo);
    const alCerrar = (error: Error) => {
      if (!this.cerrada) this.alPerderConexion?.(error);
    };
    redis.onclose = alCerrar;
    bloqueo.onclose = alCerrar;
  }

  static async conectar(url: string, worker: string, maxIntentos: number, prefijo = PREFIJO): Promise<Cola> {
    const redis = new RedisClient(url);
    try {
      await redis.connect();
      return new Cola(redis, await redis.duplicate(), worker, maxIntentos, prefijo);
    } catch (e) {
      redis.close();
      throw e;
    }
  }

  /** BLMOVE necesita Redis 6.2 o superior. Devuelve la versión. */
  async comprobarVersion(): Promise<string> {
    const info = String(await this.redis.send("INFO", ["server"]));
    const version = /^redis_version:(\S+)/m.exec(info)?.[1] ?? "";
    validarVersion(version);
    return version;
  }

  /** Registra el latido de este worker con la hora del servidor Redis. */
  async latir(): Promise<void> {
    await this.redis.eval(LATIR, 1, this.claves.workers, this.worker);
  }

  /**
   * Espera hasta `esperaS` segundos a que haya un trabajo y lo pasa a la lista
   * de este worker, gastando un intento. Devuelve null si no llegó ninguno.
   */
  async tomar(esperaS: number): Promise<TrabajoTomado | TrabajoDescartado | null> {
    const procesando = this.claves.procesando(this.worker);
    const id = await this.bloqueo.blmove(this.claves.cola, procesando, "RIGHT", "LEFT", esperaS);
    if (id === null) return null;
    const r = (await this.redis.eval(EMPEZAR, 3, this.claves.trabajo(id), procesando, this.claves.estados, id, this.worker)) as
      | [string | null, number]
      | null;
    if (!r) return { id: Number(id), descartado: true };
    return { id: Number(id), peticion: r[0] ?? "", intentos: Number(r[1]) };
  }

  /** Ids que siguen en la lista de este worker. */
  async propios(): Promise<number[]> {
    return (await this.redis.lrange(this.claves.procesando(this.worker), 0, -1)).map(Number);
  }

  /**
   * Publica el resultado. Devuelve false si el trabajo ya no era de este
   * worker (otro lo recuperó tras caducar el latido) o se canceló.
   */
  async completar(id: number, resultado: Resultado): Promise<boolean> {
    const r = await this.redis.eval(COMPLETAR, 3, this.claves.trabajo(id), this.claves.procesando(this.worker), this.claves.estados,
      String(id), this.worker, JSON.stringify(resultado), TTL_LAPIDA_MS);
    return r === 1;
  }

  /**
   * Registra un fallo. Si `reintentar` y quedan intentos, el trabajo vuelve al
   * final de la cola; si no, queda en error con `mensaje`. Devuelve el nuevo
   * estado, o null si el trabajo ya no era de este worker o se canceló.
   */
  async fallar(id: number, mensaje: string, reintentar: boolean): Promise<"pendiente" | "error" | null> {
    const r = await this.redis.eval(FALLAR, 4, this.claves.trabajo(id), this.claves.procesando(this.worker), this.claves.cola,
      this.claves.estados, String(id), this.worker, mensaje, reintentar ? "1" : "0", this.maxIntentos, TTL_LAPIDA_MS);
    return r === "pendiente" || r === "error" ? r : null;
  }

  /** Apagado: devuelve el trabajo al principio de la cola sin gastar el intento. */
  async devolver(id: number): Promise<boolean> {
    const r = await this.redis.eval(DEVOLVER, 4, this.claves.trabajo(id), this.claves.procesando(this.worker), this.claves.cola,
      this.claves.estados, String(id), this.worker);
    return r === 1;
  }

  /**
   * Trabajos de workers cuyo latido caducó (murieron): vuelven al final de la
   * cola si les quedan intentos y, si no, pasan a error.
   */
  async recuperarCaducados(caducadoMs: number): Promise<{ recuperados: number[]; agotados: number[] }> {
    const [recuperados, agotados] = (await this.redis.eval(RECUPERAR, 3, this.claves.workers, this.claves.cola, this.claves.estados,
      caducadoMs, this.maxIntentos, this.worker, this.claves.prefijo, TTL_LAPIDA_MS,
      `La simulación se interrumpió ${this.maxIntentos} veces y no se volverá a intentar.`)) as [string[], string[]];
    return { recuperados: recuperados.map(Number), agotados: agotados.map(Number) };
  }

  /** Apagado: se borra de los workers vivos si ya no tiene trabajos. */
  async baja(): Promise<boolean> {
    const r = await this.redis.eval(BAJA, 2, this.claves.workers, this.claves.procesando(this.worker), this.worker);
    return r === 1;
  }

  cerrar(): void {
    this.cerrada = true;
    this.bloqueo.close();
    this.redis.close();
  }
}

export interface OpcionesProcesador {
  trabajosSimultaneos: number;
  intervaloLatidoMs: number;
  latidoCaducadoMs: number;
  /** Cada cuánto se buscan trabajos de workers con el latido caducado. */
  intervaloRecuperacionMs?: number;
  /** Cuánto bloquea cada espera de trabajo antes de volver a mirar si hay que parar. */
  esperaTomarS?: number;
  registrar: Registrar;
}

/** Mensaje para el jugador cuando falla algo que no es culpa de su petición. */
const MENSAJE_ERROR_INTERNO = "No se pudo completar la simulación por un error interno. Inténtalo de nuevo más tarde.";

/**
 * Bucle del worker: toma trabajos mientras haya hueco, los simula en el pool
 * de hilos, mantiene el latido y recupera los trabajos de workers muertos.
 */
export class Procesador {
  private readonly enCurso = new Map<number, Promise<void>>();
  private detenido = true;
  private despertador: (() => void) | null = null;
  private bucle: Promise<void> | null = null;
  private temporizadores: Timer[] = [];
  /** Huérfanos propios vistos en la última recuperación. */
  private sospechosos = new Set<number>();

  constructor(
    private readonly cola: Cola,
    private readonly pool: PoolHilos,
    private readonly opciones: OpcionesProcesador,
  ) {}

  get trabajosEnCurso(): number {
    return this.enCurso.size;
  }

  get idsEnCurso(): number[] {
    return [...this.enCurso.keys()];
  }

  iniciar(): void {
    if (!this.detenido) return;
    this.detenido = false;
    this.temporizadores.push(setInterval(() => void this.latir(), this.opciones.intervaloLatidoMs));
    const cadaRecuperacion = this.opciones.intervaloRecuperacionMs ?? 60_000;
    this.temporizadores.push(setInterval(() => void this.recuperar(), cadaRecuperacion));
    this.bucle = this.bucleTomar();
  }

  /** Deja de tomar trabajos. Los que están en curso siguen hasta terminar. */
  async detener(): Promise<void> {
    this.detenido = true;
    this.despertar();
    await this.bucle;
  }

  /**
   * Espera a que terminen los trabajos en curso, como mucho `ms`. Devuelve
   * los ids de los que no terminaron.
   */
  async esperarEnCurso(ms: number): Promise<number[]> {
    let temporizador: Timer | undefined;
    const limite = new Promise<void>((r) => { temporizador = setTimeout(r, ms); });
    await Promise.race([Promise.allSettled([...this.enCurso.values()]), limite]);
    clearTimeout(temporizador);
    return this.idsEnCurso;
  }

  /** Para los temporizadores de latido y recuperación. */
  pararTemporizadores(): void {
    for (const t of this.temporizadores) clearInterval(t);
    this.temporizadores = [];
  }

  /** Interrumpe la espera actual del bucle (hueco libre o apagado). */
  private despertar(): void {
    this.despertador?.();
  }

  private async esperar(ms: number): Promise<void> {
    await new Promise<void>((resolver) => {
      const t = setTimeout(resolver, ms);
      this.despertador = () => { clearTimeout(t); resolver(); };
    });
    this.despertador = null;
  }

  private async bucleTomar(): Promise<void> {
    // El primer latido va antes de tomar nada: si el worker muere con un
    // trabajo, otro lo verá caducado y lo recuperará.
    await this.latir();
    void this.recuperar();
    while (!this.detenido) {
      if (this.enCurso.size >= this.opciones.trabajosSimultaneos) {
        // Lo despierta el fin de un trabajo o detener().
        await this.esperar(60_000);
        continue;
      }
      try {
        const trabajo = await this.cola.tomar(this.opciones.esperaTomarS ?? 1);
        if (!trabajo) continue;
        if ("descartado" in trabajo) {
          this.opciones.registrar("trabajo_cancelado", { id: trabajo.id });
          continue;
        }
        if (this.detenido) {
          // Lo tomamos justo al apagar: se devuelve sin gastar el intento.
          await this.cola.devolver(trabajo.id);
          break;
        }
        this.lanzar(trabajo);
      } catch (e) {
        this.opciones.registrar("error_cola", { mensaje: (e as Error).message });
        await this.esperar(1000);
      }
    }
  }

  private lanzar(trabajo: TrabajoTomado): void {
    this.opciones.registrar("trabajo_tomado", { id: trabajo.id, intento: trabajo.intentos });
    const promesa = this.procesar(trabajo)
      .catch((e) => this.opciones.registrar("error_interno", { id: trabajo.id, mensaje: (e as Error).message }))
      .finally(() => {
        this.enCurso.delete(trabajo.id);
        this.despertar();
      });
    this.enCurso.set(trabajo.id, promesa);
  }

  private async procesar(trabajo: TrabajoTomado): Promise<void> {
    const inicio = performance.now();
    const { id } = trabajo;
    const ms = () => Math.round(performance.now() - inicio);
    try {
      const preparada = prepararPeticion(leerPeticion(trabajo.peticion));
      const resultados = await Promise.all(preparada.candidatos.map((candidato) => this.pool.ejecutar({
        gen: preparada.gen, candidato, rival: preparada.rival, n: preparada.n, opciones: preparada.opciones,
      })));
      const resultado = construirResultado(preparada, resultados, performance.now() - inicio);
      if (await this.cola.completar(id, resultado)) {
        this.opciones.registrar("trabajo_completado", { id, ms: ms() });
      } else {
        this.opciones.registrar("trabajo_cancelado", { id, ms: ms() });
      }
    } catch (e) {
      const esDePeticion = e instanceof ErrorPeticion;
      const mensaje = (e as Error).message;
      const estado = await this.cola.fallar(id, esDePeticion ? mensaje : MENSAJE_ERROR_INTERNO, !esDePeticion);
      if (estado === null) {
        this.opciones.registrar("trabajo_cancelado", { id, ms: ms() });
        return;
      }
      this.opciones.registrar("trabajo_error", {
        id, ms: ms(), mensaje, peticion_invalida: esDePeticion, reintento: estado === "pendiente",
      });
    }
  }

  private async latir(): Promise<void> {
    try {
      await this.cola.latir();
    } catch (e) {
      this.opciones.registrar("error_latido", { mensaje: (e as Error).message });
    }
  }

  private async recuperar(): Promise<void> {
    try {
      const { recuperados, agotados } = await this.cola.recuperarCaducados(this.opciones.latidoCaducadoMs);
      for (const id of recuperados) this.opciones.registrar("trabajo_recuperado", { id });
      for (const id of agotados) {
        this.opciones.registrar("trabajo_error", { id, mensaje: "intentos agotados tras latidos caducados" });
      }
      await this.recuperarPropios();
    } catch (e) {
      this.opciones.registrar("error_recuperacion", { mensaje: (e as Error).message });
    }
  }

  /**
   * Trabajos que siguen en la lista de este worker pero ya no se simulan
   * (falló Redis al publicar su final). Con el latido por worker nadie más los
   * recuperaría, así que se dan por fallidos: vuelven al final de la cola. Se
   * espera a verlos en dos pasadas seguidas para no tocar uno recién tomado.
   */
  private async recuperarPropios(): Promise<void> {
    const huerfanos = (await this.cola.propios()).filter((id) => !this.enCurso.has(id));
    const confirmados = huerfanos.filter((id) => this.sospechosos.has(id));
    this.sospechosos = new Set(huerfanos);
    for (const id of confirmados) {
      if (await this.cola.fallar(id, MENSAJE_ERROR_INTERNO, true)) this.opciones.registrar("trabajo_recuperado", { id, propio: true });
    }
  }
}

/** La petición llega como texto JSON desde el hash del trabajo. */
function leerPeticion(texto: string): Peticion {
  try {
    return JSON.parse(texto);
  } catch {
    throw new ErrorPeticion("La petición guardada no es un JSON válido.");
  }
}
