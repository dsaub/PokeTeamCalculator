// Cola de trabajos en MariaDB. Go inserta los trabajos; el worker solo los
// toma (SELECT ... FOR UPDATE SKIP LOCKED) y los actualiza.

import mysql from "mysql2/promise";
import type { Pool as PoolHilos } from "./pool";
import { construirResultado, prepararPeticion } from "./sim/simular";
import { ErrorPeticion, type Peticion, type Resultado } from "./sim/tipos";

export type Registrar = (evento: string, datos?: Record<string, unknown>) => void;

export interface TrabajoTomado {
  id: number;
  /** Columna JSON tal cual: mysql2 puede devolverla como texto o ya parseada. */
  peticion: unknown;
  intentos: number;
}

export class ErrorVersion extends Error {
  override name = "ErrorVersion";
}

/** Lanza ErrorVersion si `version` (de SELECT VERSION()) no es MariaDB 10.6 o superior. */
export function validarVersion(version: string): void {
  // Algunos clientes/proxies anteponen "5.5.5-" a la versión real de MariaDB.
  const limpia = version.replace(/^5\.5\.5-/, "");
  const m = /^(\d+)\.(\d+)\./.exec(limpia);
  const mayor = Number(m?.[1] ?? 0), menor = Number(m?.[2] ?? 0);
  if (!m || !/mariadb/i.test(version) || mayor < 10 || (mayor === 10 && menor < 6)) {
    throw new ErrorVersion(`Se necesita MariaDB 10.6 o superior (para SKIP LOCKED) y el servidor es "${version}".`);
  }
}

/** Tiempo máximo que espera /salud a que MariaDB responda. */
const TIMEOUT_SALUD_MS = 2000;

/** Acceso a la tabla simulaciones. Todas las consultas van parametrizadas. */
export class Cola {
  constructor(
    readonly bd: mysql.Pool,
    readonly worker: string,
    private readonly maxIntentos: number,
  ) {}

  static conectar(url: string): mysql.Pool {
    return mysql.createPool({ uri: url, connectionLimit: 10, dateStrings: true, supportBigNumbers: true });
  }

  /** SKIP LOCKED necesita MariaDB 10.6 o superior. Devuelve la versión. */
  async comprobarVersion(): Promise<string> {
    const [filas] = await this.bd.query<mysql.RowDataPacket[]>("SELECT VERSION() AS version");
    const version = String(filas[0]?.version ?? "");
    validarVersion(version);
    return version;
  }

  /** ¿Responde MariaDB? Si tarda más de `timeoutMs`, cuenta como que no. */
  async responde(timeoutMs = TIMEOUT_SALUD_MS): Promise<boolean> {
    let temporizador: Timer | undefined;
    const limite = new Promise<false>((r) => { temporizador = setTimeout(() => r(false), timeoutMs); });
    const consulta = this.bd.query("SELECT 1").then(() => true, () => false);
    try {
      return await Promise.race([consulta, limite]);
    } finally {
      clearTimeout(temporizador);
    }
  }

  /** Toma el trabajo pendiente más antiguo y lo marca como procesando. */
  async tomar(): Promise<TrabajoTomado | null> {
    const conexion = await this.bd.getConnection();
    try {
      await conexion.beginTransaction();
      const [filas] = await conexion.query<mysql.RowDataPacket[]>(
        `SELECT id, peticion, intentos FROM simulaciones
         WHERE estado = 'pendiente' ORDER BY creado, id LIMIT 1 FOR UPDATE SKIP LOCKED`,
      );
      const fila = filas[0];
      if (!fila) {
        await conexion.commit();
        return null;
      }
      await conexion.query(
        `UPDATE simulaciones
         SET estado = 'procesando', iniciado = NOW(3), latido = NOW(3), worker = ?, intentos = intentos + 1
         WHERE id = ? AND estado = 'pendiente'`,
        [this.worker, fila.id],
      );
      await conexion.commit();
      return { id: Number(fila.id), peticion: fila.peticion, intentos: Number(fila.intentos) + 1 };
    } catch (e) {
      await conexion.rollback().catch(() => {});
      throw e;
    } finally {
      conexion.release();
    }
  }

  async estado(id: number): Promise<string | null> {
    const [filas] = await this.bd.query<mysql.RowDataPacket[]>("SELECT estado FROM simulaciones WHERE id = ?", [id]);
    return filas[0]?.estado ?? null;
  }

  async latir(ids: number[]): Promise<void> {
    if (!ids.length) return;
    await this.bd.query(
      "UPDATE simulaciones SET latido = NOW(3) WHERE id IN (?) AND estado = 'procesando' AND worker = ?",
      [ids, this.worker],
    );
  }

  /**
   * Guarda el resultado. Devuelve false si el trabajo ya no estaba en proceso
   * (cancelado) o si otro worker lo recuperó tras caducar nuestro latido.
   */
  async completar(id: number, resultado: Resultado): Promise<boolean> {
    const [r] = await this.bd.query<mysql.ResultSetHeader>(
      `UPDATE simulaciones SET estado = 'completado', resultado = ?, error = NULL, terminado = NOW(3)
       WHERE id = ? AND estado = 'procesando' AND worker = ?`,
      [JSON.stringify(resultado), id, this.worker],
    );
    return r.affectedRows > 0;
  }

  /**
   * Registra un fallo. Si `reintentar` y quedan intentos, vuelve a pendiente;
   * si no, queda en error con `mensaje`. Devuelve el nuevo estado, o null si
   * el trabajo ya no estaba en proceso (cancelado).
   */
  async fallar(id: number, mensaje: string, reintentar: boolean): Promise<"pendiente" | "error" | null> {
    if (reintentar) {
      const [r] = await this.bd.query<mysql.ResultSetHeader>(
        `UPDATE simulaciones SET estado = 'pendiente', latido = NULL, worker = NULL
         WHERE id = ? AND estado = 'procesando' AND worker = ? AND intentos < ?`,
        [id, this.worker, this.maxIntentos],
      );
      if (r.affectedRows > 0) return "pendiente";
    }
    const [r] = await this.bd.query<mysql.ResultSetHeader>(
      `UPDATE simulaciones SET estado = 'error', error = ?, terminado = NOW(3)
       WHERE id = ? AND estado = 'procesando' AND worker = ?`,
      [mensaje, id, this.worker],
    );
    return r.affectedRows > 0 ? "error" : null;
  }

  /** Apagado: devuelve a pendiente un trabajo de este worker sin gastar el intento. */
  async devolver(id: number): Promise<boolean> {
    const [r] = await this.bd.query<mysql.ResultSetHeader>(
      `UPDATE simulaciones SET estado = 'pendiente', intentos = GREATEST(intentos - 1, 0), latido = NULL, worker = NULL
       WHERE id = ? AND estado = 'procesando' AND worker = ?`,
      [id, this.worker],
    );
    return r.affectedRows > 0;
  }

  /**
   * Trabajos en proceso cuyo latido caducó (su worker murió): vuelven a
   * pendiente si les quedan intentos y, si no, pasan a error.
   */
  async recuperarCaducados(caducadoMs: number): Promise<{ recuperados: number[]; agotados: number[] }> {
    const conexion = await this.bd.getConnection();
    try {
      await conexion.beginTransaction();
      const [filas] = await conexion.query<mysql.RowDataPacket[]>(
        `SELECT id, intentos FROM simulaciones
         WHERE estado = 'procesando' AND COALESCE(latido, iniciado, creado) < NOW(3) - INTERVAL ? MICROSECOND
         FOR UPDATE SKIP LOCKED`,
        [caducadoMs * 1000],
      );
      const recuperados: number[] = [];
      const agotados: number[] = [];
      for (const f of filas) (Number(f.intentos) < this.maxIntentos ? recuperados : agotados).push(Number(f.id));
      if (recuperados.length) {
        await conexion.query(
          "UPDATE simulaciones SET estado = 'pendiente', latido = NULL, worker = NULL WHERE id IN (?)",
          [recuperados],
        );
      }
      if (agotados.length) {
        await conexion.query(
          "UPDATE simulaciones SET estado = 'error', error = ?, terminado = NOW(3) WHERE id IN (?)",
          [`La simulación se interrumpió ${this.maxIntentos} veces y no se volverá a intentar.`, agotados],
        );
      }
      await conexion.commit();
      return { recuperados, agotados };
    } catch (e) {
      await conexion.rollback().catch(() => {});
      throw e;
    } finally {
      conexion.release();
    }
  }
}

export interface OpcionesProcesador {
  trabajosSimultaneos: number;
  intervaloSondeoMs: number;
  intervaloLatidoMs: number;
  latidoCaducadoMs: number;
  /** Cada cuánto se buscan trabajos con el latido caducado. */
  intervaloRecuperacionMs?: number;
  registrar: Registrar;
  /** Se llama cuando un trabajo queda completado o en error (para avisar a Go). */
  alTerminar?: (id: number, estado: "completado" | "error") => void;
}

/** Mensaje para el jugador cuando falla algo que no es culpa de su petición. */
const MENSAJE_ERROR_INTERNO = "No se pudo completar la simulación por un error interno. Inténtalo de nuevo más tarde.";

/**
 * Bucle del worker: toma trabajos mientras haya hueco, los simula en el pool
 * de hilos, mantiene sus latidos y recupera los trabajos abandonados.
 */
export class Procesador {
  private readonly enCurso = new Map<number, Promise<void>>();
  private detenido = true;
  private despertador: (() => void) | null = null;
  private bucle: Promise<void> | null = null;
  private temporizadores: Timer[] = [];

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
    void this.recuperar();
    this.bucle = this.bucleTomar();
  }

  /** Interrumpe la espera del sondeo y busca trabajos al instante. */
  despertar(): void {
    this.despertador?.();
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

  private async bucleTomar(): Promise<void> {
    while (!this.detenido) {
      let esperaMs = this.opciones.intervaloSondeoMs;
      try {
        while (!this.detenido && this.enCurso.size < this.opciones.trabajosSimultaneos) {
          const trabajo = await this.cola.tomar();
          if (!trabajo) break;
          if (this.detenido) {
            // Lo tomamos justo al apagar: se devuelve sin gastar el intento.
            await this.cola.devolver(trabajo.id);
            break;
          }
          this.lanzar(trabajo);
        }
      } catch (e) {
        this.opciones.registrar("error_cola", { mensaje: (e as Error).message });
        esperaMs = Math.max(esperaMs, 1000);
      }
      if (this.detenido) break;
      await new Promise<void>((resolver) => {
        const t = setTimeout(resolver, esperaMs);
        this.despertador = () => { clearTimeout(t); resolver(); };
      });
      this.despertador = null;
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
      if ((await this.cola.estado(id)) === "cancelado") {
        this.opciones.registrar("trabajo_cancelado", { id });
        return;
      }
      const preparada = prepararPeticion(leerPeticion(trabajo.peticion));
      const resultados = await Promise.all(preparada.candidatos.map((candidato) => this.pool.ejecutar({
        gen: preparada.gen, candidato, rival: preparada.rival, n: preparada.n, opciones: preparada.opciones,
      })));
      const resultado = construirResultado(preparada, resultados, performance.now() - inicio);
      if (await this.cola.completar(id, resultado)) {
        this.opciones.registrar("trabajo_completado", { id, ms: ms() });
        this.opciones.alTerminar?.(id, "completado");
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
      if (estado === "error") this.opciones.alTerminar?.(id, "error");
      else this.despertar();
    }
  }

  private async latir(): Promise<void> {
    try {
      await this.cola.latir(this.idsEnCurso);
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
        this.opciones.alTerminar?.(id, "error");
      }
      if (recuperados.length) this.despertar();
    } catch (e) {
      this.opciones.registrar("error_recuperacion", { mensaje: (e as Error).message });
    }
  }
}

/** La columna JSON puede llegar como texto (MariaDB) o ya parseada. */
function leerPeticion(valor: unknown): Peticion {
  if (typeof valor === "string") {
    try {
      return JSON.parse(valor);
    } catch {
      throw new ErrorPeticion("La petición guardada no es un JSON válido.");
    }
  }
  if (valor instanceof Buffer) return leerPeticion(valor.toString("utf8"));
  return valor as Peticion;
}

export interface OpcionesCallback {
  urlCallback: string;
  tokenInterno: string;
  registrar: Registrar;
  timeoutMs?: number;
  reintentos?: number;
  /** Espera antes del primer reintento; se duplica en cada uno. */
  esperaBaseMs?: number;
}

/**
 * Avisa a Go de que el trabajo terminó. Nunca lanza: si falla del todo lo
 * registra y sigue, porque Go también sondea la base de datos.
 */
export async function avisarGo(id: number, o: OpcionesCallback): Promise<boolean> {
  const url = `${o.urlCallback}/interno/simulaciones/${id}/lista`;
  const reintentos = o.reintentos ?? 3;
  let ultimoError = "";
  for (let intento = 0; intento <= reintentos; intento++) {
    if (intento > 0) await Bun.sleep((o.esperaBaseMs ?? 500) * 2 ** (intento - 1));
    try {
      const r = await fetch(url, {
        method: "POST",
        headers: { Authorization: `Bearer ${o.tokenInterno}` },
        signal: AbortSignal.timeout(o.timeoutMs ?? 3000),
      });
      if (r.ok) return true;
      ultimoError = `HTTP ${r.status}`;
    } catch (e) {
      ultimoError = (e as Error).message;
    }
  }
  o.registrar("callback_fallido", { id, intentos: reintentos + 1, mensaje: ultimoError });
  return false;
}
