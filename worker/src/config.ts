// Variables de entorno. Se leen y validan una sola vez al arrancar.

import { isIP } from "node:net";

export interface Config {
  databaseUrl: string;
  tokenInterno: string;
  /** Sin barra final: "http://127.0.0.1:8081". */
  urlCallback: string;
  /** IP en la que escucha el servidor interno. */
  host: string;
  puerto: number;
  hilos: number;
  trabajosSimultaneos: number;
  intervaloSondeoMs: number;
  intervaloLatidoMs: number;
  latidoCaducadoMs: number;
  maxIntentos: number;
  timeoutTareaMs: number;
  esperaApagadoMs: number;
}

export class ErrorConfig extends Error {
  override name = "ErrorConfig";
}

const LONGITUD_MINIMA_TOKEN = 16;

type Entorno = Record<string, string | undefined>;

function hilosPorDefecto(): number {
  return Math.max(1, (navigator.hardwareConcurrency || 1) - 1);
}

/**
 * Lee la configuración de `entorno` (por defecto, process.env). Si algo falta
 * o es inválido lanza ErrorConfig con todos los problemas a la vez.
 */
export function leerConfig(entorno: Entorno = process.env): Config {
  const problemas: string[] = [];

  function obligatoria(nombre: string): string {
    const valor = entorno[nombre]?.trim();
    if (!valor) {
      problemas.push(`${nombre} es obligatoria.`);
      return "";
    }
    return valor;
  }

  function entero(nombre: string, porDefecto: number, minimo: number, maximo = Number.MAX_SAFE_INTEGER): number {
    const texto = entorno[nombre]?.trim();
    if (!texto) return porDefecto;
    const valor = Number(texto);
    if (!/^\d+$/.test(texto) || valor < minimo || valor > maximo) {
      problemas.push(`${nombre} debe ser un número entero entre ${minimo} y ${maximo} (es "${texto}").`);
      return porDefecto;
    }
    return valor;
  }

  function url(nombre: string, protocolos: string[], ejemplo: string): string {
    const texto = obligatoria(nombre);
    if (!texto) return "";
    try {
      const u = new URL(texto);
      if (!protocolos.includes(u.protocol)) throw new Error();
    } catch {
      problemas.push(`${nombre} no es una URL válida (por ejemplo, ${ejemplo}).`);
    }
    return texto;
  }

  const databaseUrl = url("DATABASE_URL", ["mysql:", "mariadb:"], "mysql://usuario:clave@127.0.0.1:3306/bd");
  const urlCallback = url("URL_CALLBACK", ["http:", "https:"], "http://127.0.0.1:8081").replace(/\/+$/, "");
  const host = entorno.HOST_WORKER?.trim() || "127.0.0.1";
  if (isIP(host) === 0) {
    problemas.push("HOST_WORKER debe ser una IP válida (por ejemplo 127.0.0.1 o 0.0.0.0).");
  }
  const tokenInterno = obligatoria("TOKEN_INTERNO");
  if (tokenInterno && tokenInterno.length < LONGITUD_MINIMA_TOKEN) {
    problemas.push(`TOKEN_INTERNO debe tener al menos ${LONGITUD_MINIMA_TOKEN} caracteres.`);
  }

  const config: Config = {
    databaseUrl: databaseUrl.replace(/^mariadb:/, "mysql:"),
    tokenInterno,
    urlCallback,
    host,
    puerto: entero("PUERTO", 7070, 1, 65535),
    hilos: entero("HILOS", hilosPorDefecto(), 1, 1024),
    trabajosSimultaneos: entero("TRABAJOS_SIMULTANEOS", 2, 1, 1000),
    intervaloSondeoMs: entero("INTERVALO_SONDEO_MS", 1000, 10),
    intervaloLatidoMs: entero("INTERVALO_LATIDO_MS", 5000, 100),
    latidoCaducadoMs: entero("LATIDO_CADUCADO_MS", 60000, 1000),
    maxIntentos: entero("MAX_INTENTOS", 3, 1, 100),
    timeoutTareaMs: entero("TIMEOUT_TAREA_MS", 60000, 100),
    esperaApagadoMs: entero("ESPERA_APAGADO_MS", 30000, 0),
  };

  if (!problemas.length && config.latidoCaducadoMs <= config.intervaloLatidoMs) {
    problemas.push("LATIDO_CADUCADO_MS debe ser mayor que INTERVALO_LATIDO_MS.");
  }
  if (problemas.length) throw new ErrorConfig(`Configuración inválida:\n- ${problemas.join("\n- ")}`);
  return config;
}
