// Encola un trabajo en Redis para probar el worker a mano, sin la API de Go.
// Imprime el id del trabajo.
//
//   bun herramientas/encolar.ts herramientas/ejemplo.json
//   bun herramientas/encolar.ts herramientas/ejemplo.json --id 42
//
// Usa REDIS_URL. Sin la API nadie aplica los estados que publica el worker; se
// ven con `redis-cli XRANGE sim:estados - +`.

import { RedisClient } from "bun";
import { Claves } from "../src/cola";

const args = process.argv.slice(2);
const ruta = args[0]?.startsWith("--") ? undefined : args[0];
const iId = args.indexOf("--id");
// Por defecto, un id que no choca con los de MariaDB.
const id = iId >= 0 ? Number(args[iId + 1]) : Date.now();

if (!ruta || !process.env.REDIS_URL || !Number.isSafeInteger(id) || id <= 0) {
  console.error("Uso: REDIS_URL=redis://… bun herramientas/encolar.ts <peticion.json> [--id número]");
  process.exit(1);
}

// Se guarda tal cual: validar es trabajo del worker (así se prueban también los errores).
const texto = await Bun.file(ruta).text();
const peticion = JSON.stringify(JSON.parse(texto));

const claves = new Claves();
const redis = new RedisClient(process.env.REDIS_URL);
// El mismo script que usa la API para encolar (api/endpoints/simulaciones/cola.go).
const r = await redis.eval(`
if redis.call('EXISTS', KEYS[1]) == 1 then return -1 end
redis.call('HSET', KEYS[1], 'peticion', ARGV[2], 'intentos', ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return redis.call('LPUSH', KEYS[2], ARGV[1])`, 2, claves.trabajo(id), claves.cola, String(id), peticion, 0, 24 * 60 * 60 * 1000);
redis.close();

if (r === -1) {
  console.error(`El trabajo ${id} ya existe en Redis.`);
  process.exit(1);
}
console.log(id);
