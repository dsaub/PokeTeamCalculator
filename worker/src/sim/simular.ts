import { Battle, Dex, Teams, toID } from "pokemon-showdown";
import type { PokemonSet } from "pokemon-showdown/dist/sim/teams";
import { aTextoExportacion } from "./exportar";
import { elegirMejorMovimiento, elegirMovimientoLider } from "./ia";
import { ErrorPeticion, type Peticion, type Resultado, type ResultadoCandidato } from "./tipos";

// Validación y simulación. Todo es puro y síncrono: no sabe nada de hilos,
// base de datos ni red.

export type { PokemonSet };

export const LIMITE_SIMULACIONES = 2000;
export const LIMITE_CANDIDATOS = 6;
export const LIMITE_TURNOS = 1000;

export interface OpcionesResueltas {
  aleatoriedadRival: number;
  maxTurnos: number;
}

export interface PeticionPreparada {
  /** Número de generación (1–9). */
  gen: number;
  /** Combates por candidato. */
  n: number;
  rival: PokemonSet;
  candidatos: PokemonSet[];
  opciones: OpcionesResueltas;
  /** Textos de exportación generados, útiles para depurar. */
  textos: { rival: string; candidatos: string[] };
}

export interface ResultadoCombate {
  ganador: "jugador" | "rival" | "empate";
  /** PS restantes del jugador (0–1). */
  psJugador: number;
  turnos: number;
}

type EntradaDex = { exists: boolean; gen: number; name: string; isNonstandard?: string | null };

/** No se admiten Pokémon, movimientos u objetos inventados por la comunidad. */
const NO_OFICIALES = new Set(["CAP", "Custom", "Pokestar"]);

function comprobarEnGen(entrada: EntradaDex, original: string, gen: number, quien: string): void {
  if (!entrada.exists || (entrada.isNonstandard && NO_OFICIALES.has(entrada.isNonstandard))) {
    throw new ErrorPeticion(`${quien}: ${JSON.stringify(original)} no existe.`);
  }
  if (entrada.gen > gen) throw new ErrorPeticion(`${quien}: ${JSON.stringify(entrada.name)} no existe en gen ${gen}.`);
}

/** Importa un texto de exportación y lo valida para la generación pedida. */
export function leerPokemon(texto: string, gen: number, quien: string): PokemonSet {
  const equipo = Teams.import(texto);
  if (!equipo || equipo.length !== 1) throw new ErrorPeticion(`${quien}: no se pudo leer el Pokémon.`);
  const set = equipo[0]!;
  const dex = Dex.forGen(gen);

  const especie = dex.species.get(set.species);
  comprobarEnGen(especie, set.species, gen, quien);
  set.species = especie.name;

  for (const m of set.moves) comprobarEnGen(dex.moves.get(m), m, gen, quien);
  if (set.item) comprobarEnGen(dex.items.get(set.item), set.item, gen, quien);
  if (set.nature) comprobarEnGen(dex.natures.get(set.nature), set.nature, gen, quien);

  if (set.ability) {
    const habilidad = dex.abilities.get(set.ability);
    comprobarEnGen(habilidad, set.ability, gen, quien);
    if (!Object.values(especie.abilities).includes(habilidad.name)) {
      throw new ErrorPeticion(`${quien}: ${especie.name} no puede tener la habilidad ${habilidad.name} en gen ${gen}.`);
    }
  } else if (gen >= 3) {
    set.ability = especie.abilities["0"];
  }

  if (!set.level) set.level = 100;
  return set;
}

function esObjeto(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** Valida la petición completa y convierte cada Pokémon en un set de Showdown. */
export function prepararPeticion(p: Peticion): PeticionPreparada {
  const entrada: unknown = p;
  if (!esObjeto(entrada)) throw new ErrorPeticion("La petición no tiene el formato correcto.");

  if (typeof entrada.gen !== "string" || !/^gen[1-9]$/.test(entrada.gen)) {
    throw new ErrorPeticion('La generación debe ser "gen1" … "gen9".');
  }
  const gen = Number(entrada.gen.slice(3));

  const n = entrada.simulaciones;
  if (typeof n !== "number" || !Number.isInteger(n) || n < 1 || n > LIMITE_SIMULACIONES) {
    throw new ErrorPeticion(`El número de simulaciones debe ser un entero entre 1 y ${LIMITE_SIMULACIONES}.`);
  }

  const lista = entrada.candidatos;
  if (!Array.isArray(lista) || lista.length < 1 || lista.length > LIMITE_CANDIDATOS) {
    throw new ErrorPeticion(`Debes indicar entre 1 y ${LIMITE_CANDIDATOS} Pokémon de tu equipo.`);
  }

  const opciones: OpcionesResueltas = { aleatoriedadRival: 0.2, maxTurnos: 100 };
  if (entrada.opciones !== undefined && entrada.opciones !== null) {
    if (!esObjeto(entrada.opciones)) throw new ErrorPeticion("Las opciones no tienen el formato correcto.");
    const { aleatoriedadRival, maxTurnos } = entrada.opciones;
    if (aleatoriedadRival !== undefined && aleatoriedadRival !== null) {
      if (typeof aleatoriedadRival !== "number" || !(aleatoriedadRival >= 0 && aleatoriedadRival <= 1)) {
        throw new ErrorPeticion("La aleatoriedad del rival debe estar entre 0 y 1.");
      }
      opciones.aleatoriedadRival = aleatoriedadRival;
    }
    if (maxTurnos !== undefined && maxTurnos !== null) {
      if (typeof maxTurnos !== "number" || !Number.isInteger(maxTurnos) || maxTurnos < 1 || maxTurnos > LIMITE_TURNOS) {
        throw new ErrorPeticion(`El máximo de turnos debe ser un entero entre 1 y ${LIMITE_TURNOS}.`);
      }
      opciones.maxTurnos = maxTurnos;
    }
  }

  const textoRival = aTextoExportacion(entrada.rival, "Rival");
  const rival = leerPokemon(textoRival, gen, "Rival");
  const textosCandidatos: string[] = [];
  const candidatos = lista.map((c, i) => {
    const quien = `Candidato ${i + 1}`;
    const texto = aTextoExportacion(c, quien);
    textosCandidatos.push(texto);
    return leerPokemon(texto, gen, quien);
  });

  return { gen, n, rival, candidatos, opciones, textos: { rival: textoRival, candidatos: textosCandidatos } };
}

/** Un combate 1 contra 1. El jugador usa la mejor IA y el rival la imperfecta. */
export function simularCombate(
  gen: number, jugador: PokemonSet, rival: PokemonSet, opciones: OpcionesResueltas,
): ResultadoCombate {
  const battle = new Battle({ formatid: toID(`gen${gen}customgame`) });
  // Showdown modifica los sets que recibe: siempre copias.
  battle.setPlayer("p1", { name: "jugador", team: [structuredClone(jugador)] });
  battle.setPlayer("p2", { name: "rival", team: [structuredClone(rival)] });

  try {
    // Red de seguridad: si una vuelta entera pasa sin que nadie elija ni avance
    // el turno, o se dan demasiadas vueltas, el bucle se rompe.
    const maxVueltas = opciones.maxTurnos * 10 + 20;
    let vueltas = 0;
    while (!battle.ended && battle.turn <= opciones.maxTurnos) {
      if (++vueltas > maxVueltas) break;
      const turnoAntes = battle.turn;
      let alguienEligio = false;
      for (const lado of ["p1", "p2"] as const) {
        if (battle.ended) break;
        const side = battle[lado];
        if (!side.requestState || side.isChoiceDone()) continue;
        let eleccion = "default";
        if (side.requestState === "move") {
          eleccion = lado === "p1"
            ? elegirMejorMovimiento(battle, lado)
            : elegirMovimientoLider(battle, lado, opciones.aleatoriedadRival);
        }
        if (!battle.choose(lado, eleccion)) battle.choose(lado, "default");
        alguienEligio = true;
      }
      if (!alguienEligio && battle.turn === turnoAntes) break;
    }

    const pokemon = battle.p1.pokemon[0]!;
    const psJugador = pokemon.maxhp ? pokemon.hp / pokemon.maxhp : 0;
    const turnos = battle.turn;
    let ganador: ResultadoCombate["ganador"] = "empate";
    if (battle.ended && (battle.winner === "jugador" || battle.winner === "rival")) ganador = battle.winner;
    return { ganador, psJugador, turnos };
  } finally {
    // También se libera si un manejador de Showdown lanza a mitad del combate.
    battle.destroy();
  }
}

/** `n` combates de un candidato contra el rival y su resumen. */
export function simularCandidato(
  gen: number, candidato: PokemonSet, rival: PokemonSet, n: number, opciones: OpcionesResueltas,
): ResultadoCandidato {
  let victorias = 0, derrotas = 0, empates = 0, sumaPs = 0, sumaTurnos = 0;
  for (let i = 0; i < n; i++) {
    const r = simularCombate(gen, candidato, rival, opciones);
    sumaTurnos += r.turnos;
    if (r.ganador === "jugador") { victorias++; sumaPs += r.psJugador; }
    else if (r.ganador === "rival") derrotas++;
    else empates++;
  }
  return {
    pokemon: candidato.species,
    nivel: candidato.level,
    victorias,
    derrotas,
    empates,
    winrate: victorias / n,
    psMedioVictoria: victorias ? sumaPs / victorias : 0,
    turnosMedio: sumaTurnos / n,
  };
}

/** De mejor a peor: winrate, PS medios en victoria (mayor) y turnos (menor). */
export function ordenarResultados(resultados: ResultadoCandidato[]): ResultadoCandidato[] {
  return [...resultados].sort((a, b) =>
    b.winrate - a.winrate || b.psMedioVictoria - a.psMedioVictoria || a.turnosMedio - b.turnosMedio);
}

/** "Cranidos (Nv. 14)" */
export function nombreRival(rival: PokemonSet): string {
  return `${rival.species} (Nv. ${rival.level})`;
}

/** Construye el Resultado que verá Go. */
export function construirResultado(
  preparada: PeticionPreparada, resultados: ResultadoCandidato[], ms: number,
): Resultado {
  return {
    gen: `gen${preparada.gen}`,
    rival: nombreRival(preparada.rival),
    simulaciones: preparada.n,
    resultados: ordenarResultados(resultados),
    ms: Math.round(ms),
  };
}

/** Toda la petición en un solo hilo. La usa el CLI. */
export function simularPeticion(p: Peticion): Resultado {
  const inicio = performance.now();
  const preparada = prepararPeticion(p);
  const resultados = preparada.candidatos.map((c) =>
    simularCandidato(preparada.gen, c, preparada.rival, preparada.n, preparada.opciones));
  return construirResultado(preparada, resultados, performance.now() - inicio);
}
