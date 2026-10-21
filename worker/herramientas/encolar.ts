// Inserta un trabajo en la tabla simulaciones para probar el worker a mano,
// sin la API de Go. Imprime el id del trabajo.
//
//   bun herramientas/encolar.ts herramientas/ejemplo.json
//   bun herramientas/encolar.ts herramientas/ejemplo.json --usuario ana --despertar
//
// Usa DATABASE_URL y, con --despertar, también PUERTO y TOKEN_INTERNO para
// llamar a POST /despertar del worker.

import type { ResultSetHeader } from "mysql2/promise";
import { Cola } from "../src/cola";

const args = process.argv.slice(2);
const ruta = args.find((a) => !a.startsWith("--"));
const iUsuario = args.indexOf("--usuario");
const usuario = iUsuario >= 0 ? args[iUsuario + 1] ?? "pruebas" : "pruebas";

if (!ruta || !process.env.DATABASE_URL) {
  console.error("Uso: DATABASE_URL=mysql://… bun herramientas/encolar.ts <peticion.json> [--usuario nombre] [--despertar]");
  process.exit(1);
}

// Se guarda tal cual: validar es trabajo del worker (así se prueban también los errores).
const texto = await Bun.file(ruta).text();
const peticion = JSON.stringify(JSON.parse(texto));
const hash = new Bun.CryptoHasher("sha256").update(peticion).digest("hex");

const bd = Cola.conectar(process.env.DATABASE_URL);
try {
  const [r] = await bd.query<ResultSetHeader>(
    "INSERT INTO simulaciones (hash, usuario, peticion) VALUES (?, ?, ?)",
    [hash, usuario, peticion],
  );
  console.log(r.insertId);
} finally {
  await bd.end();
}

if (args.includes("--despertar")) {
  const puerto = process.env.PUERTO || "7070";
  const r = await fetch(`http://127.0.0.1:${puerto}/despertar`, {
    method: "POST",
    headers: { Authorization: `Bearer ${process.env.TOKEN_INTERNO ?? ""}` },
  }).catch((e: Error) => e);
  if (r instanceof Error || !r.ok) console.error(`No se pudo despertar al worker: ${r instanceof Error ? r.message : `HTTP ${r.status}`}`);
}
