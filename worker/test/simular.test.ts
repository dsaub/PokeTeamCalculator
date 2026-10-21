import { describe, expect, test } from "bun:test";
import {
  leerPokemon, ordenarResultados, prepararPeticion, simularCombate, simularPeticion,
} from "../src/sim/simular";
import { ErrorPeticion, type Peticion, type ResultadoCandidato } from "../src/sim/tipos";
import ejemplo from "../herramientas/ejemplo.json";

function peticion(cambios: Partial<Peticion>): Peticion {
  return { ...(structuredClone(ejemplo) as Peticion), ...cambios };
}

function mensajeDeError(fn: () => unknown): string {
  try {
    fn();
  } catch (e) {
    expect(e).toBeInstanceOf(ErrorPeticion);
    return (e as Error).message;
  }
  throw new Error("se esperaba un ErrorPeticion");
}

describe("simulador", () => {
  test("el ejemplo da el orden esperado", () => {
    const r = simularPeticion(peticion({ simulaciones: 40 }));
    expect(r.gen).toBe("gen4");
    expect(r.rival).toBe("Cranidos (Nv. 14)");
    expect(r.simulaciones).toBe(40);
    expect(r.resultados).toHaveLength(6);
    const arriba = r.resultados.slice(0, 3).map((x) => x.pokemon).sort();
    expect(arriba).toEqual(["Grotle", "Monferno", "Prinplup"]);
    for (const x of r.resultados.slice(0, 3)) expect(x.winrate).toBeGreaterThan(0.85);
    for (const x of r.resultados.slice(3)) expect(x.winrate).toBeLessThan(0.15);
    for (const x of r.resultados) expect(x.victorias + x.derrotas + x.empates).toBe(40);
  });

  test("funciona en gen 1, 4 y 9 (gen 9 con vista previa de equipos)", () => {
    const casos: [string, Peticion["rival"], Peticion["candidatos"]][] = [
      ["gen1", { especie: "Onix", nivel: 14, movimientos: ["Tackle", "Bind"] },
        [{ especie: "Squirtle", nivel: 14, movimientos: ["Water Gun", "Tackle"] }]],
      ["gen4", { especie: "Cranidos", nivel: 14, movimientos: ["Headbutt"] },
        [{ especie: "Monferno", nivel: 16, movimientos: ["Mach Punch", "Ember"] }]],
      ["gen9", { especie: "Tinkatink", nivel: 24, movimientos: ["Fairy Wind", "Metal Claw"] },
        [{ especie: "Fuecoco", nivel: 24, movimientos: ["Ember", "Bite"] }]],
    ];
    for (const [gen, rival, candidatos] of casos) {
      const r = simularPeticion({ gen, simulaciones: 20, rival, candidatos });
      expect(r.resultados[0]!.winrate).toBeGreaterThan(0.8);
      expect(r.resultados[0]!.empates).toBe(0);
    }
  });

  test("un Pokémon con solo movimientos de estado gana cuando el rival tiene que usar Struggle", () => {
    const p = prepararPeticion({
      gen: "gen4",
      simulaciones: 1,
      rival: { especie: "Pidgey", nivel: 5, movimientos: ["Sky Attack"] },
      candidatos: [{ especie: "Shuckle", nivel: 100, movimientos: ["Harden", "Withdraw"] }],
      opciones: { maxTurnos: 1000, aleatoriedadRival: 0 },
    });
    for (let i = 0; i < 5; i++) {
      const r = simularCombate(p.gen, p.candidatos[0]!, p.rival, p.opciones);
      expect(r.ganador).toBe("jugador");
      expect(r.turnos).toBeLessThan(100);
    }
  });

  test("si se alcanza el máximo de turnos es empate y el bucle termina", () => {
    const p = prepararPeticion({
      gen: "gen4",
      simulaciones: 1,
      rival: { especie: "Shuckle", nivel: 100, movimientos: ["Harden"] },
      candidatos: [{ especie: "Shuckle", nivel: 100, movimientos: ["Withdraw"] }],
      opciones: { maxTurnos: 10 },
    });
    const r = simularCombate(p.gen, p.candidatos[0]!, p.rival, p.opciones);
    expect(r.ganador).toBe("empate");
    expect(r.turnos).toBe(11);
  });

  test("errores de generación", () => {
    expect(mensajeDeError(() => leerPokemon("Sylveon\nLevel: 50\n- Tackle", 4, "Candidato 1")))
      .toBe('Candidato 1: "Sylveon" no existe en gen 4.');
    expect(mensajeDeError(() => prepararPeticion(peticion({
      candidatos: [{ especie: "Sylveon", movimientos: ["Tackle"] }],
    })))).toBe('Candidato 1: "Sylveon" no existe en gen 4.');
    expect(mensajeDeError(() => leerPokemon("Pikachu\n- Moonblast", 4, "Rival")))
      .toBe('Rival: "Moonblast" no existe en gen 4.');
    expect(mensajeDeError(() => leerPokemon("Pikachu @ Oran Berry\n- Tackle", 2, "Rival")))
      .toBe('Rival: "Oran Berry" no existe en gen 2.');
    expect(mensajeDeError(() => leerPokemon("Pikachu\nAbility: Blaze\n- Tackle", 4, "Rival")))
      .toContain("no puede tener la habilidad Blaze");
  });

  test("pone la primera habilidad de la especie desde gen 3", () => {
    expect(leerPokemon("Gengar\n- Lick", 4, "Rival").ability).toBe("Levitate");
    expect(leerPokemon("Gengar\n- Lick", 7, "Rival").ability).toBe("Cursed Body");
    expect(leerPokemon("Gengar\n- Lick", 2, "Rival").ability).toBe("");
  });

  test("valida la petición", () => {
    expect(mensajeDeError(() => prepararPeticion(peticion({ gen: "gen10" })))).toContain("gen1");
    expect(mensajeDeError(() => prepararPeticion(peticion({ simulaciones: 0 })))).toContain("simulaciones");
    expect(mensajeDeError(() => prepararPeticion(peticion({ simulaciones: 2001 })))).toContain("simulaciones");
    expect(mensajeDeError(() => prepararPeticion(peticion({ candidatos: [] })))).toContain("entre 1 y 6");
    expect(mensajeDeError(() => prepararPeticion(peticion({ opciones: { aleatoriedadRival: 1.5 } })))).toContain("aleatoriedad");
    expect(mensajeDeError(() => prepararPeticion(null as unknown as Peticion))).toContain("formato");
  });

  test("prepararPeticion aplica valores por defecto y devuelve los textos", () => {
    const p = prepararPeticion({
      gen: "gen4", simulaciones: 5,
      rival: { especie: "Cranidos", movimientos: ["Headbutt"] },
      candidatos: [{ especie: "Monferno", movimientos: ["Ember"] }],
    });
    expect(p.gen).toBe(4);
    expect(p.n).toBe(5);
    expect(p.opciones).toEqual({ aleatoriedadRival: 0.2, maxTurnos: 100 });
    expect(p.textos.rival).toBe("Cranidos\nLevel: 100\n- Headbutt");
    expect(p.candidatos[0]!.ability).toBe("Blaze");
  });

  test("ordenarResultados: winrate, luego PS y luego turnos", () => {
    const r = (pokemon: string, winrate: number, psMedioVictoria: number, turnosMedio: number): ResultadoCandidato =>
      ({ pokemon, nivel: 1, victorias: 0, derrotas: 0, empates: 0, winrate, psMedioVictoria, turnosMedio });
    const orden = ordenarResultados([r("a", 0.5, 0.9, 3), r("b", 0.9, 0.1, 9), r("c", 0.5, 0.9, 2), r("d", 0.5, 1, 9)]);
    expect(orden.map((x) => x.pokemon)).toEqual(["b", "d", "c", "a"]);
  });
});
