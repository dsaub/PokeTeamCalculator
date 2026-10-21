import { Dex } from "pokemon-showdown";
import { ErrorPeticion } from "./tipos";

// Convierte un PokemonJson en el texto de exportación de Showdown. Nunca se
// copia texto del usuario tal cual: todo nombre se resuelve con Dex y se
// escribe su forma canónica, así que no se pueden colar líneas extra.

const CLAVES_STAT = ["hp", "atk", "def", "spa", "spd", "spe"] as const;
type ClaveStat = (typeof CLAVES_STAT)[number];

const NOMBRE_STAT: Record<ClaveStat, string> = {
  hp: "HP", atk: "Atk", def: "Def", spa: "SpA", spd: "SpD", spe: "Spe",
};

/** Muestra un valor del usuario en un mensaje de error sin romperlo. */
function citar(valor: string): string {
  const corto = valor.length > 40 ? valor.slice(0, 40) + "…" : valor;
  return JSON.stringify(corto);
}

function esObjeto(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Los campos opcionales de texto aceptan ausente, null o "" como "sin valor". */
function textoOpcional(v: unknown, campo: string, quien: string): string | undefined {
  if (v === undefined || v === null) return undefined;
  if (typeof v !== "string") throw new ErrorPeticion(`${quien}: "${campo}" debe ser un texto.`);
  return v.trim() === "" ? undefined : v;
}

function leerStats(
  v: unknown, campo: "evs" | "ivs", maximo: number, quien: string,
): Partial<Record<ClaveStat, number>> {
  if (v === undefined || v === null) return {};
  const nombre = campo === "evs" ? "EVs" : "IVs";
  if (!esObjeto(v)) throw new ErrorPeticion(`${quien}: los ${nombre} deben ser un objeto con hp, atk, def, spa, spd o spe.`);
  const stats: Partial<Record<ClaveStat, number>> = {};
  for (const [clave, valor] of Object.entries(v)) {
    if (!(CLAVES_STAT as readonly string[]).includes(clave)) {
      throw new ErrorPeticion(`${quien}: ${citar(clave)} no es una stat válida en los ${nombre} (usa hp, atk, def, spa, spd o spe).`);
    }
    if (valor === undefined || valor === null) continue;
    if (typeof valor !== "number" || !Number.isInteger(valor) || valor < 0 || valor > maximo) {
      throw new ErrorPeticion(`${quien}: los ${nombre} de ${clave} deben ser un número entero entre 0 y ${maximo}.`);
    }
    stats[clave as ClaveStat] = valor;
  }
  return stats;
}

function lineaStats(stats: Partial<Record<ClaveStat, number>>, omitir: number): string | undefined {
  const partes = CLAVES_STAT
    .filter((c) => stats[c] !== undefined && stats[c] !== omitir)
    .map((c) => `${stats[c]} ${NOMBRE_STAT[c]}`);
  return partes.length ? partes.join(" / ") : undefined;
}

/**
 * Convierte un PokemonJson (sin validar) en texto de exportación de Showdown.
 * `quien` identifica al Pokémon en los mensajes de error ("Rival", "Candidato 2").
 */
export function aTextoExportacion(entrada: unknown, quien: string): string {
  if (!esObjeto(entrada)) throw new ErrorPeticion(`${quien}: falta el Pokémon o no tiene el formato correcto.`);

  // Especie
  if (typeof entrada.especie !== "string" || entrada.especie.trim() === "") {
    throw new ErrorPeticion(`${quien}: falta la especie.`);
  }
  const especie = Dex.species.get(entrada.especie);
  if (!especie.exists) throw new ErrorPeticion(`${quien}: la especie ${citar(entrada.especie)} no existe.`);

  // Nivel
  let nivel = 100;
  if (entrada.nivel !== undefined && entrada.nivel !== null) {
    if (typeof entrada.nivel !== "number" || !Number.isInteger(entrada.nivel) || entrada.nivel < 1 || entrada.nivel > 100) {
      throw new ErrorPeticion(`${quien}: el nivel debe ser un número entero entre 1 y 100.`);
    }
    nivel = entrada.nivel;
  }

  // Movimientos
  const movs = entrada.movimientos;
  if (!Array.isArray(movs) || movs.length < 1 || movs.length > 4) {
    throw new ErrorPeticion(`${quien}: debe tener entre 1 y 4 movimientos.`);
  }
  const movimientos: string[] = [];
  const vistos = new Set<string>();
  for (const m of movs) {
    if (typeof m !== "string" || m.trim() === "") throw new ErrorPeticion(`${quien}: cada movimiento debe ser un texto no vacío.`);
    const mov = Dex.moves.get(m);
    if (!mov.exists) throw new ErrorPeticion(`${quien}: el movimiento ${citar(m)} no existe.`);
    if (vistos.has(mov.id)) throw new ErrorPeticion(`${quien}: el movimiento ${mov.name} está repetido.`);
    vistos.add(mov.id);
    movimientos.push(mov.name);
  }

  // Habilidad, objeto y naturaleza
  const textoHabilidad = textoOpcional(entrada.habilidad, "habilidad", quien);
  let habilidad: string | undefined;
  if (textoHabilidad !== undefined) {
    const h = Dex.abilities.get(textoHabilidad);
    if (!h.exists) throw new ErrorPeticion(`${quien}: la habilidad ${citar(textoHabilidad)} no existe.`);
    habilidad = h.name;
  }

  const textoObjeto = textoOpcional(entrada.objeto, "objeto", quien);
  let objeto: string | undefined;
  if (textoObjeto !== undefined) {
    const o = Dex.items.get(textoObjeto);
    if (!o.exists) throw new ErrorPeticion(`${quien}: el objeto ${citar(textoObjeto)} no existe.`);
    objeto = o.name;
  }

  const textoNaturaleza = textoOpcional(entrada.naturaleza, "naturaleza", quien);
  let naturaleza: string | undefined;
  if (textoNaturaleza !== undefined) {
    const n = Dex.natures.get(textoNaturaleza);
    if (!n.exists) throw new ErrorPeticion(`${quien}: la naturaleza ${citar(textoNaturaleza)} no existe.`);
    naturaleza = n.name;
  }

  // EVs e IVs
  const evs = leerStats(entrada.evs, "evs", 252, quien);
  const totalEvs = Object.values(evs).reduce((a, b) => a + b, 0);
  if (totalEvs > 510) throw new ErrorPeticion(`${quien}: los EVs suman ${totalEvs} y el máximo es 510.`);
  const ivs = leerStats(entrada.ivs, "ivs", 31, quien);

  // Texto. Los EVs a 0 y los IVs a 31 son los valores por defecto de Showdown.
  const lineas = [objeto ? `${especie.name} @ ${objeto}` : especie.name];
  if (habilidad) lineas.push(`Ability: ${habilidad}`);
  lineas.push(`Level: ${nivel}`);
  const lineaEvs = lineaStats(evs, 0);
  if (lineaEvs) lineas.push(`EVs: ${lineaEvs}`);
  if (naturaleza) lineas.push(`${naturaleza} Nature`);
  const lineaIvs = lineaStats(ivs, 31);
  if (lineaIvs) lineas.push(`IVs: ${lineaIvs}`);
  for (const m of movimientos) lineas.push(`- ${m}`);
  return lineas.join("\n");
}
