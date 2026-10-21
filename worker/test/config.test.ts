import { describe, expect, test } from "bun:test";
import { ErrorConfig, leerConfig } from "../src/config";

const minimo = {
  DATABASE_URL: "mysql://usuario:clave@127.0.0.1:3306/poke",
  TOKEN_INTERNO: "un-token-bastante-largo",
  URL_CALLBACK: "http://127.0.0.1:8081/",
};

function mensajeDeError(entorno: Record<string, string>): string {
  try {
    leerConfig(entorno);
  } catch (e) {
    expect(e).toBeInstanceOf(ErrorConfig);
    return (e as Error).message;
  }
  throw new Error("se esperaba un ErrorConfig");
}

describe("config", () => {
  test("valores por defecto", () => {
    const c = leerConfig(minimo);
    expect(c).toEqual({
      databaseUrl: minimo.DATABASE_URL,
      tokenInterno: minimo.TOKEN_INTERNO,
      urlCallback: "http://127.0.0.1:8081",
      host: "127.0.0.1",
      puerto: 7070,
      hilos: Math.max(1, navigator.hardwareConcurrency - 1),
      trabajosSimultaneos: 2,
      intervaloSondeoMs: 1000,
      intervaloLatidoMs: 5000,
      latidoCaducadoMs: 60000,
      maxIntentos: 3,
      timeoutTareaMs: 60000,
      esperaApagadoMs: 30000,
    });
  });

  test("lee los valores indicados", () => {
    const c = leerConfig({ ...minimo, PUERTO: "9000", HILOS: "4", TRABAJOS_SIMULTANEOS: "5", MAX_INTENTOS: "1", ESPERA_APAGADO_MS: "0" });
    expect(c.puerto).toBe(9000);
    expect(c.hilos).toBe(4);
    expect(c.trabajosSimultaneos).toBe(5);
    expect(c.maxIntentos).toBe(1);
    expect(c.esperaApagadoMs).toBe(0);
  });

  test("las vacías cuentan como no definidas", () => {
    expect(leerConfig({ ...minimo, PUERTO: "" }).puerto).toBe(7070);
  });

  test("faltan las obligatorias", () => {
    const m = mensajeDeError({});
    expect(m).toContain("DATABASE_URL es obligatoria");
    expect(m).toContain("TOKEN_INTERNO es obligatoria");
    expect(m).toContain("URL_CALLBACK es obligatoria");
  });

  test("valores inválidos", () => {
    expect(mensajeDeError({ ...minimo, PUERTO: "abc" })).toContain("PUERTO");
    expect(mensajeDeError({ ...minimo, PUERTO: "70000" })).toContain("PUERTO");
    expect(mensajeDeError({ ...minimo, HILOS: "0" })).toContain("HILOS");
    expect(mensajeDeError({ ...minimo, HILOS: "2.5" })).toContain("HILOS");
    expect(mensajeDeError({ ...minimo, MAX_INTENTOS: "-1" })).toContain("MAX_INTENTOS");
    expect(mensajeDeError({ ...minimo, DATABASE_URL: "postgres://x@y/z" })).toContain("DATABASE_URL");
    expect(mensajeDeError({ ...minimo, URL_CALLBACK: "127.0.0.1:8081" })).toContain("URL_CALLBACK");
    expect(mensajeDeError({ ...minimo, HOST_WORKER: "no-es-una-ip" })).toContain("HOST_WORKER");
    expect(mensajeDeError({ ...minimo, TOKEN_INTERNO: "corto" })).toContain("16 caracteres");
    expect(mensajeDeError({ ...minimo, INTERVALO_LATIDO_MS: "60000", LATIDO_CADUCADO_MS: "60000" }))
      .toContain("LATIDO_CADUCADO_MS debe ser mayor");
  });
});
