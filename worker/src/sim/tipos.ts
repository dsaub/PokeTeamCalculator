// Contrato con la API en Go. Go genera la Peticion y lee el Resultado de la
// base de datos: no cambies estas formas sin cambiar también el lado de Go.

/** Valores por stat. Los que falten se toman como 0 (EVs) o 31 (IVs). */
export interface Stats { hp?: number; atk?: number; def?: number; spa?: number; spd?: number; spe?: number; }

export interface PokemonJson {
  especie: string;          // nombre en inglés, como en Showdown: "Monferno", "Mr. Mime"
  nivel?: number;           // 1–100; por defecto 100
  movimientos: string[];    // 1–4, en inglés: "Mach Punch"
  habilidad?: string;       // por defecto, la primera de la especie (gen 3+)
  objeto?: string;
  naturaleza?: string;      // en inglés: "Adamant"
  evs?: Stats;              // 0–252 por stat, 510 en total
  ivs?: Stats;              // 0–31 por stat
}

export interface OpcionesSimulacion {
  aleatoriedadRival?: number;  // 0–1, por defecto 0.2
  maxTurnos?: number;          // por defecto 100
}

export interface Peticion {
  gen: string;              // "gen1" … "gen9"
  simulaciones: number;     // combates por candidato, 1–2000
  rival: PokemonJson;       // un solo Pokémon
  candidatos: PokemonJson[];// 1–6 Pokémon del jugador
  opciones?: OpcionesSimulacion;
}

export interface ResultadoCandidato {
  pokemon: string;
  nivel: number;
  victorias: number;
  derrotas: number;
  empates: number;
  winrate: number;          // victorias / simulaciones
  psMedioVictoria: number;  // PS restantes medios (0–1) en los combates ganados
  turnosMedio: number;
}

export interface Resultado {
  gen: string;
  rival: string;            // por ejemplo "Cranidos (Nv. 14)"
  simulaciones: number;
  resultados: ResultadoCandidato[];  // ordenados de mejor a peor
  ms: number;               // tiempo total de cálculo
}

/**
 * Error en los datos de entrada. El mensaje es apto para el jugador y el
 * worker marca el trabajo como error sin reintentar.
 */
export class ErrorPeticion extends Error {
  override name = "ErrorPeticion";
}
