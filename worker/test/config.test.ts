import { describe, expect, test } from "bun:test";
import { ErrorConfig, leerConfig } from "../src/config";

const minimo = {
  REDIS_URL: "redis://:clave@127.0.0.1:6379/0",
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
      redisUrl: minimo.REDIS_URL,
      hilos: Math.max(1, navigator.hardwareConcurrency - 1),
      trabajosSimultaneos: 2,
      intervaloLatidoMs: 5000,
      latidoCaducadoMs: 60000,
      maxIntentos: 3,
      timeoutTareaMs: 60000,
      esperaApagadoMs: 30000,
    });
  });

  test("lee los valores indicados", () => {
    const c = leerConfig({ ...minimo, HILOS: "4", TRABAJOS_SIMULTANEOS: "5", MAX_INTENTOS: "1", ESPERA_APAGADO_MS: "0" });
    expect(c.hilos).toBe(4);
    expect(c.trabajosSimultaneos).toBe(5);
    expect(c.maxIntentos).toBe(1);
    expect(c.esperaApagadoMs).toBe(0);
  });

  test("acepta rediss:// para Redis con TLS", () => {
    expect(leerConfig({ REDIS_URL: "rediss://:clave@redis.example:6380" }).redisUrl).toBe("rediss://:clave@redis.example:6380");
  });

  test("las vacías cuentan como no definidas", () => {
    expect(leerConfig({ ...minimo, HILOS: "" }).hilos).toBe(Math.max(1, navigator.hardwareConcurrency - 1));
  });

  test("falta la obligatoria", () => {
    expect(mensajeDeError({})).toContain("REDIS_URL es obligatoria");
  });

  test("valores inválidos", () => {
    expect(mensajeDeError({ ...minimo, HILOS: "0" })).toContain("HILOS");
    expect(mensajeDeError({ ...minimo, HILOS: "2.5" })).toContain("HILOS");
    expect(mensajeDeError({ ...minimo, MAX_INTENTOS: "-1" })).toContain("MAX_INTENTOS");
    expect(mensajeDeError({ ...minimo, REDIS_URL: "mysql://x@y/z" })).toContain("REDIS_URL");
    expect(mensajeDeError({ ...minimo, REDIS_URL: "127.0.0.1:6379" })).toContain("REDIS_URL");
    expect(mensajeDeError({ ...minimo, INTERVALO_LATIDO_MS: "60000", LATIDO_CADUCADO_MS: "60000" }))
      .toContain("LATIDO_CADUCADO_MS debe ser mayor");
  });

  test("límites superiores e inferiores", () => {
    const casos: [string, string, boolean][] = [
      ["HILOS", "1024", true], ["HILOS", "1025", false],
      ["TRABAJOS_SIMULTANEOS", "1000", true], ["TRABAJOS_SIMULTANEOS", "1001", false], ["TRABAJOS_SIMULTANEOS", "0", false],
      ["MAX_INTENTOS", "100", true], ["MAX_INTENTOS", "101", false], ["MAX_INTENTOS", "0", false],
      ["INTERVALO_LATIDO_MS", "100", true], ["INTERVALO_LATIDO_MS", "99", false],
      ["LATIDO_CADUCADO_MS", "1000", true], ["LATIDO_CADUCADO_MS", "999", false],
      ["TIMEOUT_TAREA_MS", "100", true], ["TIMEOUT_TAREA_MS", "99", false],
      ["ESPERA_APAGADO_MS", "0", true], ["ESPERA_APAGADO_MS", "-5", false], ["ESPERA_APAGADO_MS", "abc", false],
    ];
    for (const [nombre, valor, valido] of casos) {
      const entorno = { ...minimo, INTERVALO_LATIDO_MS: "100", [nombre]: valor };
      if (valido) expect(() => leerConfig(entorno), `${nombre}=${valor}`).not.toThrow();
      else expect(mensajeDeError(entorno), `${nombre}=${valor}`).toContain(`${nombre} debe ser un número entero`);
    }
  });

  test("el mensaje dice el rango y el valor recibido", () => {
    expect(mensajeDeError({ ...minimo, HILOS: "2000" })).toContain('HILOS debe ser un número entero entre 1 y 1024 (es "2000").');
  });

  test("recorta los espacios", () => {
    const c = leerConfig({ REDIS_URL: "  redis://127.0.0.1:6379  ", HILOS: " 4 " });
    expect(c.redisUrl).toBe("redis://127.0.0.1:6379");
    expect(c.hilos).toBe(4);
  });

  test("REDIS_URL con solo espacios cuenta como vacía", () => {
    expect(mensajeDeError({ REDIS_URL: "   " })).toContain("REDIS_URL es obligatoria");
  });

  test("informa de todos los problemas a la vez, en líneas", () => {
    const m = mensajeDeError({ HILOS: "x", MAX_INTENTOS: "0" });
    expect(m).toStartWith("Configuración inválida:\n- ");
    expect(m).toContain("REDIS_URL es obligatoria.");
    expect(m).toContain("HILOS");
    expect(m).toContain("MAX_INTENTOS");
    expect(m.split("\n- ")).toHaveLength(4);
  });

  test("no compara los latidos si ya hay otros problemas", () => {
    const m = mensajeDeError({ ...minimo, HILOS: "0", INTERVALO_LATIDO_MS: "5000", LATIDO_CADUCADO_MS: "1000" });
    expect(m).not.toContain("LATIDO_CADUCADO_MS debe ser mayor");
  });

  test("latido caducado menor que el intervalo", () => {
    expect(mensajeDeError({ ...minimo, INTERVALO_LATIDO_MS: "5000", LATIDO_CADUCADO_MS: "1000" })).toContain("LATIDO_CADUCADO_MS debe ser mayor");
  });

  test("sin hardwareConcurrency usa un hilo", () => {
    const original = Object.getOwnPropertyDescriptor(navigator, "hardwareConcurrency");
    Object.defineProperty(navigator, "hardwareConcurrency", { value: 0, configurable: true });
    try {
      expect(leerConfig(minimo).hilos).toBe(1);
    } finally {
      if (original) Object.defineProperty(navigator, "hardwareConcurrency", original);
      else delete (navigator as { hardwareConcurrency?: number }).hardwareConcurrency;
    }
  });

  test("por defecto lee process.env", () => {
    const antes = process.env.REDIS_URL;
    process.env.REDIS_URL = "redis://desde-entorno:6379";
    try {
      expect(leerConfig().redisUrl).toBe("redis://desde-entorno:6379");
    } finally {
      if (antes === undefined) delete process.env.REDIS_URL;
      else process.env.REDIS_URL = antes;
    }
  });

  test("ErrorConfig tiene su nombre", () => {
    expect(new ErrorConfig("x").name).toBe("ErrorConfig");
  });
});
